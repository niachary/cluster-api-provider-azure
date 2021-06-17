/*
Copyright 2019 The Kubernetes Authors.

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
	"github.com/niachary/cluster-api-provider-azure/cloud/scope"
	
)

// azureClusterReconciler is the reconciler called by the AzureCluster controller
type azureIPPoolReconciler struct {
	scope     *scope.AzureIPPoolScope
}

// newAzureClusterReconciler populates all the services based on input scope
func newAzureIPPoolReconciler(scope *scope.AzureIPPoolScope) *azureIPPoolReconciler {
	return &azureIPPoolReconciler{
		scope:     scope,
	}
}

// Reconcile reconciles all the services in pre determined order
func (r *azureIPPoolReconciler) Reconcile(ctx context.Context) error {
	return nil
}

// Delete reconciles all the services in pre determined order
func (r *azureIPPoolReconciler) Delete(ctx context.Context) error {
	return nil
}

func (r *azureIPPoolReconciler) Delete(ctx context.Context) error {
	return nil
} 