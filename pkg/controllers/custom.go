package controllers

import (
	"fmt"
	"log"
	"time"

	"github.com/mfojtik/custom-deployment/pkg/informers"
	"github.com/mfojtik/custom-deployment/pkg/strategy"
	"github.com/mfojtik/custom-deployment/pkg/util/conversion"
	deployutil "github.com/mfojtik/custom-deployment/pkg/util/deployments"
	"github.com/mfojtik/custom-deployment/pkg/util/workqueue"
	typed "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

type CustomController struct {
	extensionsClient typed.ExtensionsInterface

	deploymentLister *cache.StoreToDeploymentLister
	deploymentSynced cache.InformerSynced

	strategy strategy.Interface
	queue    workqueue.RateLimitingInterface
}

func NewCustomController(dInformer informers.DeploymentInformer, extClient typed.ExtensionsInterface, strategy strategy.Interface) *CustomController {
	c := &CustomController{
		extensionsClient: extClient,
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "custom-deployment"),
		strategy:         strategy,
	}

	dInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeploymentNotification,
		UpdateFunc: c.updateDeploymentNotification,
		DeleteFunc: c.deleteDeploymentNotification,
	})

	c.deploymentLister = dInformer.Lister()
	c.deploymentSynced = dInformer.Informer().HasSynced

	return c
}

func (c *CustomController) addDeploymentNotification(obj interface{}) {
	d := obj.(*v1beta1.Deployment)
	c.enqueueDeployment(d)
}

func (c *CustomController) updateDeploymentNotification(_, newObj interface{}) {
	c.enqueueDeployment(newObj.(*v1beta1.Deployment))
}

func (c *CustomController) deleteDeploymentNotification(obj interface{}) {
	d, ok := obj.(*v1beta1.Deployment)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		d, ok = tombstone.Obj.(*v1beta1.Deployment)
		if !ok {
			return
		}
	}
	c.enqueueDeployment(d)
}

func (c *CustomController) enqueueDeployment(deployment *v1beta1.Deployment) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(deployment)
	if err != nil {
		log.Printf("Couldn't get key for object %#v: %v", deployment, err)
		return
	}

	c.queue.Add(key)
}

func (c *CustomController) Run(threadiness int, stopCh <-chan struct{}) {
	log.Printf("Starting custom deployment strategy controller")
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	if !cache.WaitForCacheSync(stopCh, c.deploymentSynced) {
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.worker, time.Second, stopCh)
	}

	<-stopCh
	log.Printf("Shutting down custom deployment strategy controller")
}

func (c *CustomController) worker() {
	for {
		if quit := c.process(); quit {
			break
		}
	}
}

// processNextWorkItem deals with one key off the queue.  It returns false when it's time to quit.
func (c *CustomController) process() bool {
	work := func() bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.handleDeployment(key.(string)); err != nil {
			utilruntime.HandleError(fmt.Errorf("%v failed with : %v", key, err))
			c.queue.AddRateLimited(key)
			return false
		}

		return false
	}

	for {
		if quit := work(); quit {
			return true
		}
	}
}

func (c *CustomController) handleDeployment(key string) error {
	obj, exists, err := c.deploymentLister.Indexer.GetByKey(key)
	if err != nil {
		log.Printf("Unable to retrieve deployment %v from store: %v", key, err)
		return err
	}

	if !exists {
		log.Printf("Deployment has been deleted %v", key)
		return nil
	}

	deployment := obj.(*v1beta1.Deployment)
	d, err := deployutil.DeploymentDeepCopy(conversion.DeploymentToInternal(deployment))
	if err != nil {
		return err
	}

	log.Printf("Executing custom strategy rollout for deployment %s/%s", d.Namespace, d.Name)
	return c.strategy.Rollout(nil, nil, nil)
}
