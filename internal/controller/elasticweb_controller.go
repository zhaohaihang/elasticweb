/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	elasticwebv1 "github.com/zhaohaihang/elasticweb/api/v1"
)

const (
	APP_NAME       = "elastic-app"
	CONTAINER_PORT = 8080
	CPU_REQUEST    = "100m"
	CPU_LIMIT      = "100m"
	MEM_REQUEST    = "512Mi"
	MEM_LIMIT      = "512Mi"
)

var logger = log.Log.WithName("elasticweb")

// ElasticWebReconciler reconciles a ElasticWeb object
type ElasticWebReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=elasticweb.com.zhh,resources=elasticwebs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticweb.com.zhh,resources=elasticwebs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elasticweb.com.zhh,resources=elasticwebs/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ElasticWeb object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *ElasticWebReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger.Info("1. start reconcile logic")

	elasticWeb := &elasticwebv1.ElasticWeb{}
	if err := r.Get(ctx, req.NamespacedName, elasticWeb); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("2.1. instance not found, maybe removed")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "2.2 error")
		return ctrl.Result{}, err
	}
	logger.Info("3. instance : " + elasticWeb.String())

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, deployment)
	if err != nil { // 如果出错了，则处理不同的错误
		if errors.IsNotFound(err) { // 如果没找到 ，则需要创建
			logger.Info("4. deployment not exists")
			if elasticWeb.Spec.TotalQPS < 1 { // 如果qps为0，说明没有请求，则不需要创建deployment
				logger.Info("5.1 not need deployment")
				return ctrl.Result{}, nil
			}

			if err := createServiceIfNotExists(ctx, r, elasticWeb, req); err != nil { // 创建deployment之前，先创建service
				logger.Error(err, "5.2 error")
				return ctrl.Result{}, nil
			}

			if err := createDeployment(ctx, r, elasticWeb); err != nil { // 创建deployment
				logger.Error(err, "5.3 error")
				return ctrl.Result{}, nil
			}

			if err := updateStatus(ctx, r, elasticWeb); err != nil { // 更新elasticweb的状态
				logger.Error(err, "5.4 error")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		} else { // 如果不是没找到，而是其他错误则返回错误
			logger.Error(err, "7. error")
			return ctrl.Result{}, err
		}
	}

	// 如果没出错，说明deployment已经有了，则进行调和
	expectReplicas := getExpectReplicas(elasticWeb)
	realReplicas := deployment.Spec.Replicas
	logger.Info("9. expectReplicas [%d], realReplicas [%d]", expectReplicas, realReplicas)

	if expectReplicas != *realReplicas { // 如果真实副本数和 期望的副本数不一致，则更新deployment
		logger.Info("11. update deployment's Replicas")
		deployment.Spec.Replicas = &expectReplicas
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("13. update status") // 更新完deployment之后，更新elasticweb的状态，主要是更新其中的realQPS
		if err := updateStatus(ctx, r, elasticWeb); err != nil {
			logger.Error(err, "14. update status error")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ElasticWebReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticwebv1.ElasticWeb{}).
		Complete(r)
}

func getExpectReplicas(elasticWeb *elasticwebv1.ElasticWeb) int32 { // 用于获取期望副本数量值
	singleQPS := elasticWeb.Spec.SinglePodQPS
	totalQPS := elasticWeb.Spec.TotalQPS

	replicas := totalQPS / singleQPS
	if totalQPS%singleQPS > 0 {
		replicas++
	}
	return replicas
}

func createDeployment(ctx context.Context, r *ElasticWebReconciler, elasticWeb *elasticwebv1.ElasticWeb) error {

	expectReplicas := getExpectReplicas(elasticWeb)
	logger.Info(fmt.Sprintf("expectReplicas [%d]", expectReplicas))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: elasticWeb.Namespace,
			Name:      elasticWeb.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &expectReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": APP_NAME,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": APP_NAME,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            APP_NAME,
							Image:           elasticWeb.Spec.Image,
							ImagePullPolicy: "IfNotPresent",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									Protocol:      corev1.ProtocolSCTP,
									ContainerPort: CONTAINER_PORT,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse(CPU_REQUEST),
									"memory": resource.MustParse(MEM_REQUEST),
								},
								Limits: corev1.ResourceList{
									"cpu":    resource.MustParse(CPU_LIMIT),
									"memory": resource.MustParse(MEM_LIMIT),
								},
							},
						},
					},
				},
			},
		},
	}

	logger.Info("set reference")
	if err := controllerutil.SetControllerReference(elasticWeb, deployment, r.Scheme); err != nil {
		logger.Error(err, "SetControllerReference error")
		return err
	}

	logger.Info("start create deployment")
	if err := r.Create(ctx, deployment); err != nil {
		logger.Error(err, "create deployment error")
		return err
	}

	logger.Info("create deployment success")

	return nil
}

func createServiceIfNotExists(ctx context.Context, r *ElasticWebReconciler, elasticWeb *elasticwebv1.ElasticWeb, req ctrl.Request) error {
	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil { //查询时有错误，要处理
		if errors.IsNotFound(err) { // 如果因为没找到sercvice报错，则需要创建service
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: elasticWeb.Namespace,
					Name:      elasticWeb.Name,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     8080,
							NodePort: elasticWeb.Spec.Port,
						},
					},
					Selector: map[string]string{
						"app": APP_NAME,
					},
					Type: corev1.ServiceTypeNodePort,
				},
			}

			logger.Info("set reference")
			if err := controllerutil.SetControllerReference(elasticWeb, service, r.Scheme); err != nil { // 建立关联关系
				logger.Error(err, "SetControllerReference error")
				return err
			}

			logger.Info("start create service")
			if err := r.Create(ctx, service); err != nil { // 创建service
				logger.Error(err, "create service error")
				return err
			}
			logger.Info("create service success")
			return nil
		}
		logger.Error(err, "query service error") // 如果不是没找service，而是其他错误，则上报
		return err
	}

	logger.Info("service exists") // 如果查询时没有错误，说明存在service就不用创建了
	return nil
}

func updateStatus(ctx context.Context, r *ElasticWebReconciler, elasticWeb *elasticwebv1.ElasticWeb) error {
	singlePodQPS := elasticWeb.Spec.SinglePodQPS
	replicas := getExpectReplicas(elasticWeb)
	elasticWeb.Status.RealQPS = singlePodQPS * replicas
	logger.Info(fmt.Sprintf("singlePodQPS [%d], replicas [%d], realQPS[%d]", singlePodQPS, replicas, elasticWeb.Status.RealQPS))

	if err := r.Update(ctx, elasticWeb); err != nil {
		logger.Error(err, "update instance error")
		return err
	}
	return nil
}
