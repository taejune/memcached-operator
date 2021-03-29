/*


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

package controllers

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/taejune/memcached-operator/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

const finalizer = "tmax.io/memcached-operatorName"

// +kubebuilder:rbac:groups=memcached.tmax.io,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.tmax.io,resources=memcacheds/status,verbs=get;update;patch

func (r *MemcachedReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("memcached", req.NamespacedName)

	// Fetch the Memcached instance
	instance := &memcachedv1alpha1.Memcached{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if instance.Status.Phase == memcachedv1alpha1.MemcachedPhaseUndefined {
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, finalizer)
		if err = r.Update(ctx, instance); err != nil {
			logger.Error(err, " failed to patch Memcached instance status")
			return ctrl.Result{}, err
		}

		instance.Status.Phase = memcachedv1alpha1.MemcachedPhasePending
		if err = r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, " failed to patch Memcached instance status")
			return ctrl.Result{}, err
		}
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("DeleteTimeStamp is NOT ZERO")
		if isContainFinalizer(instance) {
			logger.Info("Remove instance's finalizer")
			removeFinalizer(instance)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		logger.Info(">>>> >>>> END reconcile")
		return ctrl.Result{}, nil
	}

	// Define a new Pod object
	pod := newPodForCR(instance)
	// Set Memcached instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, pod, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if this Pod already exists
	found := &corev1.Pod{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err = r.Client.Create(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Pod already exists - don't requeue
	logger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name, "Pod.ResourceVersion",
		found.ResourceVersion, "Pod.Status.Phase", found.Status.Phase, "Pod.Status.Conditions", found.Status.Conditions)

	logger.Info(">>>> >>>> END reconcile")

	return ctrl.Result{}, nil
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *memcachedv1alpha1.Memcached) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}

func isContainFinalizer(m *memcachedv1alpha1.Memcached) bool {
	for _, f := range m.ObjectMeta.Finalizers {
		if f == finalizer {
			return true
		}
	}

	return false
}

func removeFinalizer(m *memcachedv1alpha1.Memcached) {
	newFinalizers := []string{}
	for _, f := range m.ObjectMeta.Finalizers {
		if f == finalizer {
			continue
		}
		newFinalizers = append(newFinalizers, f)
	}

	m.ObjectMeta.Finalizers = newFinalizers
}

func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&memcachedv1alpha1.Memcached{}).
		Complete(r)
}
