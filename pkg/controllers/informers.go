package controllers

import (
	"reflect"
	"sync"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
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

type sharedInformerFactory struct {
	client        *kubernetes.Clientset
	lock          sync.Mutex
	defaultResync time.Duration

	informers        map[reflect.Type]cache.SharedIndexInformer
	startedInformers map[reflect.Type]bool
}

// NewSharedInformerFactory constructs a new instance of sharedInformerFactory
func NewSharedInformerFactory(client *kubernetes.Clientset, defaultResync time.Duration) *sharedInformerFactory {
	return &sharedInformerFactory{
		client:           client,
		defaultResync:    defaultResync,
		informers:        make(map[reflect.Type]cache.SharedIndexInformer),
		startedInformers: make(map[reflect.Type]bool),
	}
}

func (s *sharedInformerFactory) Start(stopCh <-chan struct{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for informerType, informer := range s.informers {
		if !s.startedInformers[informerType] {
			go informer.Run(stopCh)
			s.startedInformers[informerType] = true
		}
	}
}

func (f *sharedInformerFactory) Deployments() DeploymentInformer {
	return &deploymentInformer{sharedInformerFactory: f}
}

func (f *sharedInformerFactory) ReplicaSets() ReplicaSetInformer {
	return &replicaSetInformer{sharedInformerFactory: f}
}

type deploymentInformer struct {
	*sharedInformerFactory
}

func (f *deploymentInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&v1beta1.Deployment{})
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
		&v1beta1.Deployment{},
		f.defaultResync,
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
	*sharedInformerFactory
}

func (f *replicaSetInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&v1beta1.ReplicaSet{})
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
		&v1beta1.ReplicaSet{},
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
