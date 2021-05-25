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

package virtualmachines

import (
	"context"

	//"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2020-06-01/compute"
	compute "github.com/Azure/azure-sdk-for-go/profiles/2018-03-01/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest"
	//future "github.com/Azure/go-autorest/autorest/azure"
	"k8s.io/klog/klogr"
	azure "github.com/niachary/cluster-api-provider-azure/cloud"
	"fmt"
	"time"
)

// Client wraps go-sdk
type Client interface {
	Get(context.Context, string, string) (compute.VirtualMachine, error)
	CreateOrUpdate(context.Context, string, string, compute.VirtualMachine) error
	Delete(context.Context, string, string) error
}

// AzureClient contains the Azure go-sdk Client
type AzureClient struct {
	virtualmachines compute.VirtualMachinesClient
}

var _ Client = &AzureClient{}

// NewClient creates a new VM client from subscription ID.
func NewClient(auth azure.Authorizer) *AzureClient {
	c := newVirtualMachinesClient(auth.SubscriptionID(), auth.BaseURI(), auth.Authorizer())
	return &AzureClient{c}
}

// newVirtualMachinesClient creates a new VM client from subscription ID.
func newVirtualMachinesClient(subscriptionID string, baseURI string, authorizer autorest.Authorizer) compute.VirtualMachinesClient {
	vmClient := compute.NewVirtualMachinesClientWithBaseURI(baseURI, subscriptionID)
	vmClient.Authorizer = authorizer
	vmClient.AddToUserAgent(azure.UserAgent())
	return vmClient
}

// Get retrieves information about the model view or the instance view of a virtual machine.
func (ac *AzureClient) Get(ctx context.Context, resourceGroupName, vmName string) (compute.VirtualMachine, error) {
	return ac.virtualmachines.Get(ctx, resourceGroupName, vmName, "")
}

// CreateOrUpdate the operation to create or update a virtual machine.
func (ac *AzureClient) CreateOrUpdate(ctx context.Context, resourceGroupName, vmName string, vm compute.VirtualMachine) error {
	log := klogr.New()
	log.Info("before creating VM")
	future, err := ac.virtualmachines.CreateOrUpdate(ctx, resourceGroupName, vmName, vm)

	if err != nil {
		return err
	}
	log.Info("after create or update, before waiting")
	/*err = future.WaitForCompletionRef(ctx, ac.virtualmachines.Client)
	if err != nil {
		return err
	}*/
	client := ac.virtualmachines.Client
	f := future
	WaitForCompletionRef(&f,ctx,client)
	log.Info("after waiting for completion")

	_, err = future.Result(ac.virtualmachines)
	log.Info("after waiting for completion, returning the error")
	return err
}

// Delete the operation to delete a virtual machine.
func (ac *AzureClient) Delete(ctx context.Context, resourceGroupName, vmName string) error {
	log := klogr.New()
	log.Info("before deleting the VM")
	future, err := ac.virtualmachines.Delete(ctx, resourceGroupName, vmName)
	if err != nil {
		return err
	}
	log.Info("after delete VM, before waiting")
	err = future.WaitForCompletionRef(ctx, ac.virtualmachines.Client)
	if err != nil {
		return err
	}
	log.Info("after waiting for completion")
	_, err = future.Result(ac.virtualmachines)
	log.Info("after waiting for completion, returning the error")
	return err
}


func WaitForCompletionRef(f *compute.VirtualMachinesCreateOrUpdateFuture,ctx context.Context,client autorest.Client) error {
	log := klogr.New()
	log.Info("In wait for completion Ref")
	
	defer func() {
		sc := -1
		resp := f.Response()
		if resp != nil {
			sc = resp.StatusCode
		}
		log.Info(fmt.Sprintf("printing sc %s",sc))
	}()
	cancelCtx := ctx
	_, hasDeadline := ctx.Deadline()
	if d := client.PollingDuration; !hasDeadline && d != 0 {
		log.Info(fmt.Sprintf("setting context with timeout as d %d",d))
		var cancel context.CancelFunc
		cancelCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	// if the initial response has a Retry-After, sleep for the specified amount of time before starting to poll
	if delay, ok := f.GetPollingDelay(); ok {
		log.Info(fmt.Sprintf("Delay is: %d",delay))
		if delayElapsed := autorest.DelayForBackoff(delay, 0, cancelCtx.Done()); !delayElapsed {
			err := cancelCtx.Err()
			return err
		}
	}
	log.Info(fmt.Sprintf("before the first attempt"))
	done, err := f.DoneWithContext(ctx, client)
	for attempts := 0; !done; done, err = f.DoneWithContext(ctx, client) {
		log.Info(fmt.Sprintf("attempt number %d",attempts))
		if attempts >= client.RetryAttempts {
			log.Info(fmt.Sprintf("attempts %d has exceeded retry attempts %d",attempts,client.RetryAttempts))
			return err
			//return autorest.NewErrorWithError(err, "Future", "WaitForCompletion", "the number of retries has been exceeded")
		}
		// we want delayAttempt to be zero in the non-error case so
		// that DelayForBackoff doesn't perform exponential back-off
		var delayAttempt int
		var delay time.Duration
		if err == nil {
			// check for Retry-After delay, if not present use the client's polling delay
			var ok bool
			delay, ok = f.GetPollingDelay()
			if !ok {
				log.Info(fmt.Sprintf("setting delay as clients polling delay %d",client.PollingDelay))
				delay = client.PollingDelay
			}
		} else {
			// there was an error polling for status so perform exponential
			// back-off based on the number of attempts using the client's retry
			// duration.  update attempts after delayAttempt to avoid off-by-one.
			log.Info(fmt.Sprintf("exponential backoff"))
			delayAttempt = attempts
			delay = client.RetryDuration
			attempts++
		}
		// wait until the delay elapses or the context is cancelled
		delayElapsed := autorest.DelayForBackoff(delay, delayAttempt, cancelCtx.Done())
		if !delayElapsed {
			log.Info(fmt.Sprintf("returning Error"))
			cancelCtx.Err()
			//return autorest.NewErrorWithError(cancelCtx.Err(), "Future", "WaitForCompletion", f.pt.latestResponse(), "context has been cancelled")
		}
		log.Info(fmt.Sprintf("end of for loop"))
	}
	log.Info("returning from WaitForCompletionRef")
	return err
}

