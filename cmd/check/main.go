package main

import (
	"github.com/cloudboss/ofcourse/ofcourse"
	"github.com/matthewmcnew/kpack-resource/resource"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	ofcourse.Check(&resource.Resource{})
}
