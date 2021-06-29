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
	"sync"
	azure "github.com/niachary/cluster-api-provider-azure/cloud"
)

var lock = &sync.Mutex{}

// ClusterScopeParams defines the input parameters used to create a new Scope.
type IPPoolScopeParams struct {
	Client       client.Client
	Logger       logr.Logger
	AzureIPPool *infrav1.AzureIPPool
	Lock  		sync.Mutex
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
		//Lock: params.Lock,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type AzureIPPoolScope struct {
	logr.Logger
	Client      client.Client
	patchHelper *patch.Helper
	//Lock sync.Mutex
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

func (s *AzureIPPoolScope) ReconcileIPs(ctx context.Context, machineScope *MachineScope, ippoolname string, machineIP string) error {
	log := klogr.New()
	lock.Lock()
	log.Info(fmt.Sprintf("Locking reconcile IP for machine %s", machineScope.Name()))
	defer func() {
		log.Info(fmt.Sprintf("Unlocking reconcile IP for machine %s", machineScope.Name()))
     lock.Unlock()   
    }()
	
	azureippools, err := s.GetIPPoolObj(ctx, "default", ippoolname)
    if err!=nil {
        return errors.Wrapf(err, "Failed to retrieve Azure IP pool")
    }

	for index, _ := range(azureippools.Spec.IPPools){
		ippool := &azureippools.Spec.IPPools[index]
        networkInterface := BuildNetworkInterfaceSpec(machineScope,*ippool,(*ippool).Name)
        if ippool.Name == "mgmt-nic-ip-pool" {
			log.Info(fmt.Sprintf("Free IPPool before %v", (*ippool).FreeIPs))
			allocatedIP := ""
			
			if (machineIP == "") {
				allocatedIP = (*ippool).FreeIPs[0]	
				(*ippool).FreeIPs = (*ippool).FreeIPs[1:]
				
			} else {
				foundIP := false
				for i, ip := range (*ippool).FreeIPs {
					if(ip == machineIP){
						foundIP = true
						allocatedIP = ip
						(*ippool).FreeIPs = append((*ippool).FreeIPs[0:i], (*ippool).FreeIPs[i+1:]...)
						break;
					}
				}
				
				// if ApiServer IP is not found in the free IP pool
				if foundIP == false {
					return errors.New("Failed to retrieve IP matching the APIServerIP from the free IP pool for control plane")
				}
				log.Info(fmt.Sprintf("Adding IP %s to allocated IP pool",allocatedIP))
			}

			log.Info(fmt.Sprintf("Free IPPool after %v",(*ippool).FreeIPs))
			log.Info(fmt.Sprintf("Allocated IPPool before %v", (*ippool).AllocatedIPs))
			if (*ippool).AllocatedIPs == nil {
				allocatedIPs := []string{allocatedIP}
				(*ippool).AllocatedIPs = allocatedIPs
			}else {
				(*ippool).AllocatedIPs = append((*ippool).AllocatedIPs, allocatedIP)
			}
			log.Info(fmt.Sprintf("Allocated IPPool after %v", (*ippool).AllocatedIPs))
			if err := s.Client.Update(ctx, azureippools); err != nil {
				return errors.Wrapf(err, "Failed to update Azure IP pool")
			}
			log.Info(fmt.Sprintf("Successfully updated the IP pool CR %s", azureippools.Name))

			networkInterface.StaticIPAddress = allocatedIP
        }
        machineScope.AzureMachine.Spec.NetworkInterfaces = append(machineScope.AzureMachine.Spec.NetworkInterfaces, networkInterface)
	}
	return nil
}


/*func (s *AzureIPPoolScope) AddToAllocatedIPPool(machineScope *MachineScope, ippool *infrav1.IPPool, allocatedIP string) error {
	log := klogr.New()
	log.Info(fmt.Sprintf("Adding IP %s to allocated IP pool",allocatedIP))

	

	if ippool.AllocatedIPs == nil {
		log.Info(fmt.Sprintf("AllocatedIPPool before is nil"))
		allocatedIPs := []string{allocatedIP}
		ippool.AllocatedIPs = allocatedIPs
	}else {
		log.Info(fmt.Sprintf("AllocatedIPPool before %v", ippool.AllocatedIPs))
		ippool.AllocatedIPs = append(ippool.AllocatedIPs, allocatedIP)
	}

	log.Info(fmt.Sprintf("AllocatedIPPool after %v",ippool.AllocatedIPs))
	
	

	return nil
}*/

/*func (s *AzureIPPoolScope) UpdateIPPool(ctx context.Context, azureippool *infrav1.AzureIPPool) error {
	log := klogr.New()
	//updating the AzureIPPool CRD
	if err := s.Client.Update(ctx, azureippool); err != nil {
		return errors.Wrapf(err, "Failed to update Azure IP pool")
	}
	log.Info(fmt.Sprintf("Successfully updated the IP pool CR %s", azureippool.Name))
	return nil
}*/

func (s *AzureIPPoolScope) FreeIPs(ctx context.Context, machineScope *MachineScope, ippoolname string) error {
	log := klogr.New()
	lock.Lock()
	log.Info(fmt.Sprintf("Locking Free IP for machine %s", machineScope.Name()))
	defer func() {
		log.Info(fmt.Sprintf("Unlocking Free IP for machine %s", machineScope.Name()))
    	lock.Unlock()   
    }()

	azureippools, err := s.GetIPPoolObj(ctx, "default","ase-ip-pool")
    
    if err!=nil {
        return errors.Wrapf(err, "Failed to retrieve Azure IP pool")
    }

	for index, _ := range(azureippools.Spec.IPPools) {
		ippool := &azureippools.Spec.IPPools[index]
		if((*ippool).Name == "mgmt-nic-ip-pool"){        
			log.Info("Inside free IPs for azuremachine %s",machineScope.Name())
			primaryNetworkInterface, err := GetPrimaryNetworkInterface(machineScope.AzureMachine.Spec.NetworkInterfaces)
			if err!= nil {
				return errors.Wrapf(err, "Error getting primary nic for the machine %s",machineScope.Name())
			}
			freeIP := primaryNetworkInterface.StaticIPAddress

			//Remove from allocated IP pool
			log.Info(fmt.Sprintf("Getting IP %s from allocated IP pool",freeIP))
			log.Info(fmt.Sprintf("Allocated IPPool before %v", (*ippool).AllocatedIPs))
			foundIP := false
			for i, ip := range (*ippool).AllocatedIPs {
				if(ip == freeIP){
					foundIP = true
					(*ippool).AllocatedIPs = append((*ippool).AllocatedIPs[0:i], (*ippool).AllocatedIPs[i+1:]...)
					break;
				}
			}
			if foundIP == false {
				return errors.New("Failed to get IP in the allocated IPs pool")
			}
			log.Info(fmt.Sprintf("Allocated IPPool after %v", (*ippool).AllocatedIPs))
			
			log.Info(fmt.Sprintf("Free IPPool before is %v", (*ippool).FreeIPs))
			//Adding to free IP pool
			if (*ippool).FreeIPs == nil {
				freeIPs := []string{freeIP}
				(*ippool).FreeIPs = freeIPs
			}else{
				(*ippool).FreeIPs = append((*ippool).FreeIPs, freeIP)
			}
			log.Info(fmt.Sprintf("Free IPPool after is %v", (*ippool).FreeIPs))
			

			if err := s.Client.Update(ctx, azureippools); err != nil {
				return errors.Wrapf(err, "Failed to update Azure IP pool")
			}
			log.Info(fmt.Sprintf("Successfully updated the IP pool CR %s", azureippools.Name))
		}
	}

	return nil
}

/*func (s *AzureIPPoolScope) RemoveFromAllocatedIPPool(machineScope *MachineScope, ippool *infrav1.IPPool , freeIP string) error {

	log := klogr.New()
	log.Info(fmt.Sprintf("Getting IP %s from allocated IP pool",freeIP))

	

	log.Info(fmt.Sprintf("Allocated IPPool before %v", ippool.AllocatedIPs))
	foundIP := false
	for index, ip := range ippool.AllocatedIPs {
		if(ip == freeIP){
			foundIP = true
			ippool.AllocatedIPs = append(ippool.AllocatedIPs[0:index], ippool.AllocatedIPs[index+1:]...)
			break;
		}
	}
	if foundIP == false {
		return errors.New("Failed to get IP in the allocated IPs pool")
	}

	log.Info(fmt.Sprintf("Allocated IPPool after %v", ippool.AllocatedIPs))

	

	return nil
}*/

func BuildNetworkInterfaceSpec(machineScope *MachineScope,ippool infrav1.IPPool, name string) infrav1.NetworkInterface {
    networkInterface := infrav1.NetworkInterface{}
    networkInterface.Name = azure.GenerateNICName(machineScope.Name())
    networkInterface.VnetName = ippool.VnetName
    networkInterface.SubnetName = ippool.SubnetName
    networkInterface.VnetResourceGroup = "aserg"
    if name == "mgmt-nic-ip-pool" {
        networkInterface.IsPrimary = true
    } else {
        networkInterface.StaticIPAddress = ""
		networkInterface.AcceleratedNetworking = true
    }
    return networkInterface
}

func GetPrimaryNetworkInterface(nics []infrav1.NetworkInterface) (infrav1.NetworkInterface, error) {
    nic := infrav1.NetworkInterface{}
	for _,nic := range(nics) {
        if nic.IsPrimary == true {
            return nic, nil
        }
    }
    return nic, errors.New("Unable to find primary nic for the machine")
}