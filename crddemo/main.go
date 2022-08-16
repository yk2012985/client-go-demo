package main

import (
	"context"
	clientset "crddemo/pkg/generated/clientset/versioned"
	"crddemo/pkg/generated/informers/externalversions"
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"log"
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		log.Fatal(err)
	}
	clientSet, err := clientset.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	list, err := clientSet.CrdV1().Foos("db-test").List(context.TODO(), v1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for _, foo := range list.Items {
		fmt.Println(foo.Name)
	}

	// 此处的NewSharedInformerFactory使用代码生成器生成的
	factory := externalversions.NewSharedInformerFactory(clientSet, 0)
	factory.Crd().V1().Foos().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// to do
		},
	})

}
