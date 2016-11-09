package controllers

import (
	"fmt"
	"log"
	"time"

	"github.com/mfojtik/custom-deployment/pkg/util/workqueue"
	"k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

type CustomController struct {
	extensionsClient v1beta1.ExtensionsInterface
	deploymentLister *cache.StoreToDeploymentLister
	replicaSetLister *cache.StoreToReplicaSetLister

	deploymentSynced cache.InformerSynced
	replicaSetSynced cache.InformerSynced

	queue workqueue.RateLimitingInterface
}

func NewCustomController(dInformer DeploymentInformer, rsInformer ReplicaSetInformer, extClient v1beta1.ExtensionsInterface) *CustomController {
	c := &CustomController{
		extensionsClient: extClient,
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "customdeployment"),
	}

	dInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeploymentNotification,
		UpdateFunc: c.updateDeploymentNotification,
		DeleteFunc: c.deleteDeploymentNotification,
	})

	rsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})

	c.deploymentLister = dInformer.Lister()
	c.replicaSetLister = rsInformer.Lister()

	return c
}

func (c *CustomController) addDeploymentNotification(obj interface{})               {}
func (c *CustomController) updateDeploymentNotification(oldObj, newObj interface{}) {}
func (c *CustomController) deleteDeploymentNotification(obj interface{})            {}

func (c *CustomController) Run(threadiness int, stopCh <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.ShutDown()

	log.Printf("Starting custom deployment controller")

	// wait for your secondary caches to fill before starting your work
	if !cache.WaitForCacheSync(stopCh, c.deploymentSynced) || !cache.WaitForCacheSync(stopCh, c.replicaSetSynced) {
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	// wait until we're told to stop
	<-stopCh
	log.Printf("Shutting down custom deployment controller")
}

func (c *CustomController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem deals with one key off the queue.  It returns false when it's time to quit.
func (c *CustomController) processNextWorkItem() bool {
	work := func() bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.syncDeployment(key.(string)); err != nil {
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

func (c *CustomController) syncDeployment(key string) error {
	startTime := time.Now()
	defer func() {
		log.Printf("Finished syncing deployment %q (%v)", key, time.Now().Sub(startTime))
	}()

	obj, exists, err := c.deploymentLister.Indexer.GetByKey(key)
	if err != nil {
		log.Printf("Unable to retrieve deployment %v from store: %v", key, err)
		return err
	}
	if !exists {
		log.Printf("Deployment has been deleted %v", key)
		return nil
	}

	deployment := obj.(*extensions.Deployment)
	d, err := deploymentDeepCopy(deployment)
	if err != nil {
		return err
	}

	log.Printf("Handling deployment %s/%s", d.Namespace, d.Name)

	return nil
}

func deploymentDeepCopy(deployment *extensions.Deployment) (*extensions.Deployment, error) {
	objCopy, err := api.Scheme.DeepCopy(deployment)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*extensions.Deployment)
	if !ok {
		return nil, fmt.Errorf("expected Deployment, got %#v", objCopy)
	}
	return copied, nil
}
