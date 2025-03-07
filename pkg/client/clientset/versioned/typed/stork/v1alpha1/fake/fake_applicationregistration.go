/*
Copyright 2018 Openstorage.org

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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/libopenstorage/stork/pkg/apis/stork/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeApplicationRegistrations implements ApplicationRegistrationInterface
type FakeApplicationRegistrations struct {
	Fake *FakeStorkV1alpha1
}

var applicationregistrationsResource = schema.GroupVersionResource{Group: "stork.libopenstorage.org", Version: "v1alpha1", Resource: "applicationregistrations"}

var applicationregistrationsKind = schema.GroupVersionKind{Group: "stork.libopenstorage.org", Version: "v1alpha1", Kind: "ApplicationRegistration"}

// Get takes name of the applicationRegistration, and returns the corresponding applicationRegistration object, and an error if there is any.
func (c *FakeApplicationRegistrations) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ApplicationRegistration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(applicationregistrationsResource, name), &v1alpha1.ApplicationRegistration{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationRegistration), err
}

// List takes label and field selectors, and returns the list of ApplicationRegistrations that match those selectors.
func (c *FakeApplicationRegistrations) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ApplicationRegistrationList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(applicationregistrationsResource, applicationregistrationsKind, opts), &v1alpha1.ApplicationRegistrationList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ApplicationRegistrationList{ListMeta: obj.(*v1alpha1.ApplicationRegistrationList).ListMeta}
	for _, item := range obj.(*v1alpha1.ApplicationRegistrationList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested applicationRegistrations.
func (c *FakeApplicationRegistrations) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(applicationregistrationsResource, opts))
}

// Create takes the representation of a applicationRegistration and creates it.  Returns the server's representation of the applicationRegistration, and an error, if there is any.
func (c *FakeApplicationRegistrations) Create(ctx context.Context, applicationRegistration *v1alpha1.ApplicationRegistration, opts v1.CreateOptions) (result *v1alpha1.ApplicationRegistration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(applicationregistrationsResource, applicationRegistration), &v1alpha1.ApplicationRegistration{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationRegistration), err
}

// Update takes the representation of a applicationRegistration and updates it. Returns the server's representation of the applicationRegistration, and an error, if there is any.
func (c *FakeApplicationRegistrations) Update(ctx context.Context, applicationRegistration *v1alpha1.ApplicationRegistration, opts v1.UpdateOptions) (result *v1alpha1.ApplicationRegistration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(applicationregistrationsResource, applicationRegistration), &v1alpha1.ApplicationRegistration{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationRegistration), err
}

// Delete takes name of the applicationRegistration and deletes it. Returns an error if one occurs.
func (c *FakeApplicationRegistrations) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(applicationregistrationsResource, name), &v1alpha1.ApplicationRegistration{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeApplicationRegistrations) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(applicationregistrationsResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.ApplicationRegistrationList{})
	return err
}

// Patch applies the patch and returns the patched applicationRegistration.
func (c *FakeApplicationRegistrations) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ApplicationRegistration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(applicationregistrationsResource, name, pt, data, subresources...), &v1alpha1.ApplicationRegistration{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ApplicationRegistration), err
}
