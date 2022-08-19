package pkg

import (
	"context"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v13 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	informer "k8s.io/client-go/informers/core/v1"
	netInformer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	coreLister "k8s.io/client-go/listers/core/v1"
	v1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"reflect"
	"time"
)

const (
	workerNum  = 5
	maxRetries = 10
)

type controller struct {
	client        kubernetes.Interface
	ingressLister v1.IngressLister
	serviceLister coreLister.ServiceLister

	queue workqueue.RateLimitingInterface
}

func (c *controller) Run(stopCh chan struct{}) {
	for i := 0; i < workerNum; i++ {
		go wait.Until(c.worker, time.Minute, stopCh)
	}
	<-stopCh
}

func NewController(client kubernetes.Interface, serviceInformer informer.ServiceInformer, ingressInformer netInformer.IngressInformer) controller {
	c := controller{
		client:        client,
		ingressLister: ingressInformer.Lister(),
		serviceLister: serviceInformer.Lister(),

		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingressManager"),
	}

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
	})

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.deleteIngress,
	})

	return c
}

func (c *controller) updateService(oldObj interface{}, newObj interface{}) {
	// TODO
	if reflect.DeepEqual(newObj, oldObj) {
		return
	}
	c.enqueue(newObj)
}

func (c *controller) addService(obj interface{}) {
	c.enqueue(obj)
}

func (c *controller) deleteIngress(obj interface{}) {
	ingress := obj.(v12.Ingress)
	ownerReference := v13.GetControllerOf(&ingress)
	if ownerReference == nil {
		return
	}
	if ownerReference.Kind != "Service" {
		return
	}
	c.queue.Add(ingress.Name + "/" + ingress.Namespace)
}

func (c *controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
	}
	c.queue.Add(key)
}

func (c controller) worker() {
	for c.processItem() {

	}
}

func (c *controller) processItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	key := item.(string)
	err := c.syncService(key)
	if err != nil {
		c.hanlerError(key, err)
	}

	return true
}

func (c *controller) syncService(item string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(item)
	if err != nil {
		return err
	}
	// delete Service
	service, err := c.serviceLister.Services(namespace).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	//add and delete
	_, ok := service.GetAnnotations()["ingress/http"]
	ingress, err := c.ingressLister.Ingresses(namespace).Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if ok && errors.IsNotFound(err) {
		// create ingress
		ig := c.constructIngress(namespace, name)
		_, err := c.client.NetworkingV1().Ingresses(namespace).Create(context.TODO(), ig, v13.CreateOptions{})
		if err != nil {
			return err
		}

	} else if !ok && ingress != nil {
		// delete ingress
		err := c.client.NetworkingV1beta1().Ingresses(namespace).Delete(context.TODO(), name, v13.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *controller) constructIngress(namespace string, name string) *v12.Ingress {
	ingress := v12.Ingress{}
	ingress.Name = name
	ingress.Namespace = namespace
	pathType := v12.PathTypePrefix
	ingress.Spec = v12.IngressSpec{
		Rules: []v12.IngressRule{
			{
				Host: "example.com",
				IngressRuleValue: v12.IngressRuleValue{
					HTTP: &v12.HTTPIngressRuleValue{
						Paths: []v12.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &pathType,
								Backend: v12.IngressBackend{
									Service: &v12.IngressServiceBackend{
										Name: name,
										Port: v12.ServiceBackendPort{
											Name:   name,
											Number: 80,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return &ingress
}

func (c *controller) hanlerError(key string, err error) {
	if c.queue.NumRequeues(key) <= maxRetries {
		c.queue.AddRateLimited(key)
		return
	} else {
		runtime.HandleError(err)
		c.queue.Forget(key)
	}

}
