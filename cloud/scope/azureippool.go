/*
Copyright 2018 The Kubernetes Authors.

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

package scope

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/klog/klogr"
	infrav1 "github.com/niachary/cluster-api-provider-azure/api/v1alpha3"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"fmt"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type IPPoolScopeParams struct {
	Client       client.Client
	Logger       logr.Logger
	AzureIPPool *infrav1.AzureIPPool
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewIPPoolScope(params IPPoolScopeParams) (*AzureIPPoolScope, error) {
	log := klogr.New()
	log.Info("in NewIPPoolScope")
	if params.AzureIPPool == nil {
		return nil, errors.New("failed to generate new scope from nil AzureIPPool")
	}

	if params.Logger == nil {
		params.Logger = klogr.New()
	}

	helper, err := patch.NewHelper(params.AzureIPPool, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}

	return &AzureIPPoolScope{
		Logger:       params.Logger,
		Client:       params.Client,
		AzureIPPool: params.AzureIPPool,
		patchHelper:  helper,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type AzureIPPoolScope struct {
	logr.Logger
	Client      client.Client
	patchHelper *patch.Helper

	AzureIPPool *infrav1.AzureIPPool
}

// PatchObject persists the cluster configuration and status.
func (s *AzureIPPoolScope) PatchObject(ctx context.Context) error {
	return s.patchHelper.Patch(
		ctx,
		s.AzureIPPool,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.NetworkInfrastructureReadyCondition,
		}})
}

// Close closes the current scope persisting the cluster configuration and status.
func (s *AzureIPPoolScope) Close(ctx context.Context) error {
	return s.patchHelper.Patch(ctx, s.AzureIPPool)
}

func (s *AzureIPPoolScope) GetIPPoolObj(ctx context.Context, namespace string, ipPoolName string) (*infrav1.AzureIPPool, error) {
	azureIPPoolName := client.ObjectKey{
		Namespace: namespace,
		Name:      ipPoolName,
	}
	azureippool := &infrav1.AzureIPPool{}
	if err := s.Client.Get(ctx, azureIPPoolName, azureippool); err != nil {
	return azureippool, errors.Wrapf(err, "Failed to retrieve Azure IP pool")
	}
	return azureippool, nil
}

func (s *AzureIPPoolScope) GetFromFreeIPPool(ctx context.Context, namespace string, ipPoolName string, machineIP string) (string, error) {
	log := klogr.New()
	azureippool, err := s.GetIPPoolObj(ctx, namespace, ipPoolName)
	if err!= nil {
		return "", errors.Wrapf(err, "Failed to get ip pool while getting free IP for the machine")
	}

	allocatedIP := ""
	if (machineIP == "") {
		//networkInterface.StaticIPAddress = azureippool.Spec.IPPool[0]
		allocatedIP = azureippool.Spec.IPPool[0]
		azureippool.Spec.IPPool = azureippool.Spec.IPPool[1:]
	} else {
		foundIP := false
		for index, ip := range azureippool.Spec.IPPool {
			if(ip == machineIP){
				foundIP = true
				//networkInterface.StaticIPAddress = ip
				allocatedIP = ip
				azureippool.Spec.IPPool = append(azureippool.Spec.IPPool[0:index], azureippool.Spec.IPPool[index+1:]...)
				break;
			}
		}
		// if ApiServer IP is not found in the free IP pool
		if foundIP == false {
			return "", errors.New("Failed to retrieve IP matching the APIServerIP from the free IP pool for control plane")
		}
	}

	log.Info(fmt.Sprintf("IP pool: %v", azureippool.Spec.IPPool))
	
	//updating the AzureIPPool CRD
	if err := s.Client.Update(ctx, azureippool); err != nil {
		return "", errors.Wrapf(err, "Failed to update Azure IP pool")
	}
	return allocatedIP, nil
}

func (s *AzureIPPoolScope) AddToAllocatedIPPool(ctx context.Context, namespace string, ipPoolName string, allocatedIP string) error {
	log := klogr.New()
	log.Info(fmt.Sprintf("Adding IP %s to allocated IP pool",allocatedIP))

	azureippool, err := s.GetIPPoolObj(ctx, namespace, ipPoolName)
	if err!= nil {
		return errors.Wrapf(err, "Failed to get ip pool while getting free IP for the machine")
	}

	if azureippool.Spec.AllocatedIPs == nil {
		allocatedIPs := []string{allocatedIP}
		azureippool.Spec.AllocatedIPs = allocatedIPs
	}else {
		azureippool.Spec.AllocatedIPs = append(azureippool.Spec.AllocatedIPs, allocatedIP)
	}

	log.Info(fmt.Sprintf("Allocated IP pool: %v", azureippool.Spec.AllocatedIPs))
	
	//updating the AzureIPPool CRD
	if err := s.Client.Update(ctx, azureippool); err != nil {
		return errors.Wrapf(err, "Failed to update Azure IP pool")
	}

	return nil
}

func (s *AzureIPPoolScope) AddToFreeIPPool(ctx context.Context, namespace string, ipPoolName string, freeIP string) error {

	log := klogr.New()
	log.Info(fmt.Sprintf("Getting IP %s from allocated IP pool",freeIP))

	azureippool, err := s.GetIPPoolObj(ctx, namespace, ipPoolName)
	if err!= nil {
		return errors.Wrapf(err, "Failed to get ip pool")
	}

	if azureippool.Spec.IPPool == nil {
		freeIPs := []string{freeIP}
		azureippool.Spec.IPPool = freeIPs
	}else{
		azureippool.Spec.IPPool = append(azureippool.Spec.IPPool, freeIP)
	}

	log.Info(fmt.Sprintf("Free IP pool %v", azureippool.Spec.IPPool))
	
	//updating the AzureIPPool CRD
	if err := s.Client.Update(ctx, azureippool); err != nil {
		return errors.Wrapf(err, "Failed to update Azure IP pool")
	}

	return nil
}

func (s *AzureIPPoolScope) RemoveFromAllocatedIPPool(ctx context.Context, namespace string, ipPoolName string, freeIP string) error {

	log := klogr.New()
	log.Info(fmt.Sprintf("Getting IP %s from allocated IP pool",freeIP))

	azureippool, err := s.GetIPPoolObj(ctx, namespace, ipPoolName)
	if err!= nil {
		return errors.Wrapf(err, "Failed to get ip pool")
	}

	foundIP := false
	for index, ip := range azureippool.Spec.AllocatedIPs {
		if(ip == freeIP){
			foundIP = true
			azureippool.Spec.AllocatedIPs = append(azureippool.Spec.AllocatedIPs[0:index], azureippool.Spec.AllocatedIPs[index+1:]...)
			break;
		}
	}
	if foundIP == false {
		return errors.New("Failed to get IP in the allocated IPs pool")
	}

	log.Info(fmt.Sprintf("Allocated IP pool %v", azureippool.Spec.AllocatedIPs))

	//updating the AzureIPPool CRD
	if err := s.Client.Update(ctx, azureippool); err != nil {
		return errors.Wrapf(err, "Failed to update Azure IP pool")
	}

	return nil
}


