package controllers

import (
	"reflect"
	"sync"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/tools/cache"
)

type DeploymentInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() *cache.StoreToDeploymentLister
}

type ReplicaSetInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() *cache.StoreToReplicaSetLister
}

type SharedInformerFactory struct {
	client        *kubernetes.Clientset
	lock          sync.Mutex
	defaultResync time.Duration

	informers map[reflect.Type]cache.SharedIndexInformer
	// startedInformers is used for tracking which informers have been started
	// this allows calling of Start method multiple times
	startedInformers map[reflect.Type]bool
}

// NewSharedInformerFactory constructs a new instance of sharedInformerFactory
func NewSharedInformerFactory(client *kubernetes.Clientset, defaultResync time.Duration) *SharedInformerFactory {
	return &SharedInformerFactory{
		client:           client,
		defaultResync:    defaultResync,
		informers:        make(map[reflect.Type]cache.SharedIndexInformer),
		startedInformers: make(map[reflect.Type]bool),
	}
}

func (s *SharedInformerFactory) Start(stopCh <-chan struct{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for informerType, informer := range s.informers {
		if !s.startedInformers[informerType] {
			go informer.Run(stopCh)
			s.startedInformers[informerType] = true
		}
	}
}

func (f *SharedInformerFactory) Deployments() DeploymentInformer {
	return &deploymentInformer{SharedInformerFactory: f}
}

func (f *SharedInformerFactory) ReplicaSets() ReplicaSetInformer {
	return &replicaSetInformer{SharedInformerFactory: f}
}

type deploymentInformer struct {
	*SharedInformerFactory
}

func (f *deploymentInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&extensions.Deployment{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}
	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return f.client.Extensions().Deployments(api.NamespaceAll).List(options)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return f.client.Extensions().Deployments(api.NamespaceAll).Watch(options)
			},
		},
		&extensions.Deployment{},
		// TODO remove this.  It is hardcoded so that "Waiting for the second deployment to clear overlapping annotation" in
		// "overlapping deployment should not fight with each other" will work since it requires a full resync to work properly.
		30*time.Second,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	f.informers[informerType] = informer

	return informer
}

func (f *deploymentInformer) Lister() *cache.StoreToDeploymentLister {
	informer := f.Informer()
	return &cache.StoreToDeploymentLister{Indexer: informer.GetIndexer()}
}

type replicaSetInformer struct {
	*SharedInformerFactory
}

func (f *replicaSetInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&extensions.ReplicaSet{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}
	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return f.client.Extensions().ReplicaSets(api.NamespaceAll).List(options)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return f.client.Extensions().ReplicaSets(api.NamespaceAll).Watch(options)
			},
		},
		&extensions.ReplicaSet{},
		f.defaultResync,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	f.informers[informerType] = informer

	return informer
}

func (f *replicaSetInformer) Lister() *cache.StoreToReplicaSetLister {
	informer := f.Informer()
	return &cache.StoreToReplicaSetLister{Store: informer.GetIndexer()}
}
