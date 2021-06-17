/*
Copyright The Kubernetes Authors.

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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/cluster-api/util/predicates"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"github.com/niachary/cluster-api-provider-azure/util/reconciler"
	"github.com/niachary/cluster-api-provider-azure/cloud/scope"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/niachary/cluster-api-provider-azure/api/v1alpha3"
)

// AzureIPPoolReconciler reconciles a AzureIPPool object
type AzureIPPoolReconciler struct {
	client.Client
	Log              logr.Logger
	Recorder         record.EventRecorder
	ReconcileTimeout time.Duration
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureippools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=azureippools/status,verbs=get;update;patch

func (r *AzureIPPoolReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(context.Background(), reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()
	log := r.Log.WithValues("namespace", req.Namespace, "AzureIPPool", req.Name)

	// your logic here
	azureippool := &infrav1.AzureIPPool{}
	err := r.Get(ctx, req.NamespacedName, azureippool)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("object was not found")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Create the scope.
	ipPoolScope, err := scope.NewIPPoolScope(scope.IPPoolScopeParams{
		Client:       r.Client,
		Logger:       log,
		AzureIPPool: azureippool,
	})
	if err != nil {
		return reconcile.Result{}, errors.Errorf("failed to create scope: %+v", err)
	}

	// Always close the scope when exiting this function so we can persist any AzureMachine changes.
	defer func() {
		if err := ipPoolScope.Close(ctx); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle deleted ip pools
	if !azureippool.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ipPoolScope)
	}

	// Handle non-deleted ip pools
	return r.reconcileNormal(ctx, ipPoolScope)

	return ctrl.Result{}, nil
}

func (r *AzureIPPoolReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	log := r.Log.WithValues("controller", "AzureIPPool")
	_, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1.AzureIPPool{}).
		WithEventFilter(predicates.ResourceNotPaused(log)). // don't queue reconcile if resource is paused
		Build(r)
	if err != nil {
		return errors.Wrapf(err, "error creating controller")
	}

	/*// Add a watch on clusterv1.Cluster object for unpause notifications.
	if err = c.Watch(
		&source.Kind{Type: &infrav1.AzureIPPool{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: util.ClusterToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("AzureCluster")),
		},
		predicates.ClusterUnpaused(log),
	); err != nil {
		return errors.Wrapf(err, "failed adding a watch for ready clusters")
	}*/

	return nil
}

func (r *AzureIPPoolReconciler) reconcileNormal(ctx context.Context, ipPoolScope *scope.AzureIPPoolScope) (reconcile.Result, error) {
	ipPoolScope.Info("Reconciling AzureIPPool")
	azureIPPool := ipPoolScope.AzureIPPool

	// If the AzureIPPool doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(azureIPPool, infrav1.IPPoolFinalizer)
	// Register the finalizer immediately to avoid orphaning Azure resources on delete
	if err := ipPoolScope.PatchObject(ctx); err != nil {
		return reconcile.Result{}, err
	}

	err := newAzureIPPoolReconciler(ipPoolScope).Reconcile(ctx)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile ip pool services")
	}

	return reconcile.Result{}, nil
}

func (r *AzureIPPoolReconciler) reconcileDelete(ctx context.Context, ipPoolScope *scope.AzureIPPoolScope) (reconcile.Result, error) {
	ipPoolScope.Info("Reconciling AzureIPPool delete")

	azureIPPool := ipPoolScope.AzureIPPool

	if err := newAzureIPPoolReconciler(ipPoolScope).Delete(ctx); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "error deleting AzureIPPool %s/%s", azureIPPool.Namespace, azureIPPool.Name)
	}

	// IPPool is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(ipPoolScope.AzureIPPool, infrav1.IPPoolFinalizer)

	return reconcile.Result{}, nil
}
