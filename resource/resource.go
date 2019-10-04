// Package resource is an implementation of a Concourse resource.
package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	oc "github.com/cloudboss/ofcourse/ofcourse"
	"github.com/pivotal/kpack/pkg/client/clientset/versioned"
	"github.com/pivotal/kpack/pkg/logs"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"knative.dev/pkg/apis/duck/v1alpha1"
	"strings"
	"time"
)

// This resource is only a skeleton for getting started. What it does:
//
// For `Check`, it increments its version each time it is called, starting with `{"count": "1"}`. The
// next version would be `{"count": "2"}`, and so on.
//
// For `In`, it writes a file containing the latest version to its output directory.
//
// For `Out`, it looks in the directory created by `In` for the version file and reads it into a map,
// returning it back to Concourse.
//
// In case there is any confusion about which directions `In` and `Out` refer to, you should implement
// `In` to read remotely and write locally, while `Out` should read locally and write remotely. `In`
// receives an output directory and should place its result there after retrieving it from some (usually)
// remote source, configured through `source` in the pipeline's resource definition. `Out` receives an
// input directory to read from, and should place its result back to the remote source.
//
// This skeleton resource does not deal with a remote source; it simply reads and writes local files.
//
// Keep in mind, this is only an example! Beware of always returning a unique version on every check
// the way this example does. If you have many such resources that are checked frequently across many
// pipelines, it will put a lot of load on the database's CPU.

var (
	// ErrVersion means version map is malformed
	ErrVersion = errors.New(`key "count" not found in version map`)
	// ErrParam means parameters are malformed
	ErrParam = errors.New(`missing "version_path" parameter`)
)

// Resource implements the ofcourse.Resource interface.
type Resource struct{}

// Check implements the ofcourse.Resource Check method, corresponding to the /opt/resource/check command.
// This is called when Concourse does its resource checks, or when the `fly check-resource` command is run.
func (r *Resource) Check(source oc.Source, version oc.Version, env oc.Environment,
	logger *oc.Logger) ([]oc.Version, error) {

	var oldVersion string
	if version != nil {
		var ok bool
		oldVersion, ok = version["ref"]
		if !ok {
			return nil, ErrVersion
		}
	}

	k, ok := source["kubeconfig"].(string)
	if !ok {
		logger.Errorf("not a string")
		return nil, errors.New("blah")
	}

	f, err := ioutil.TempFile("", "kube")
	if err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	f.WriteString(k)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	err = f.Close()
	if err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	clusterConfig, err := clientcmd.BuildConfigFromFlags("", f.Name())
	if err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	clientset, err := versioned.NewForConfig(clusterConfig)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	namespace, ok := source["namespace"].(string)
	if !ok {
		logger.Errorf("namespace not a string")
		return nil, errors.New("blah")
	}

	imageName, ok := source["image"].(string)
	if !ok {
		logger.Errorf("namespace not a string")
		return nil, errors.New("namespace not a string")
	}
	image, err := clientset.BuildV1alpha1().Images(namespace).Get(imageName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if image.Status.GetCondition(v1alpha1.ConditionReady).IsTrue() && image.Status.LatestImage != oldVersion {
		versions := []oc.Version{{
			"ref":   image.Status.LatestImage,
			"build": image.Status.LatestBuildRef,
		}}
		return versions, nil
	}

	// Returned `versions` should be all of the versions since the one given in the `version`
	// argument. If `version` is nil, then return the first available version. In many cases there
	// will be only one version to return, depending on the type of resource being implemented.
	// For example, a git resource would return a list of commits since the one given in the
	// `version` argument, whereas that would not make sense for resources which do not have any
	// kind of linear versioning.

	//count := "1"
	//if version != nil {
	//	oldCount, ok := version["count"]
	//	if !ok {
	//		return nil, ErrVersion
	//	}
	//	i, err := strconv.Atoi(oldCount)
	//	if err != nil {
	//		return nil, err
	//	}
	//	count = strconv.Itoa(i + 1)
	//}

	// In Concourse, a version is an arbitrary set of string keys and string values.
	// This particular version consists of just one key and value.

	return []oc.Version{}, nil

}

// In implements the ofcourse.Resource In method, corresponding to the /opt/resource/in command.
// This is called when a Concourse job does `get` on the resource.
func (r *Resource) In(outputDirectory string, source oc.Source, params oc.Params, version oc.Version,
	env oc.Environment, logger *oc.Logger) (oc.Version, oc.Metadata, error) {
	// Demo of logging. Resources should never use fmt.Printf or anything that writes
	// to standard output, as it will corrupt the JSON output expected by Concourse.

	// Write the `version` argument to a file in the output directory,
	// so the `Out` function can read it.
	outputPath := fmt.Sprintf("%s/version", outputDirectory)
	bytes, err := json.Marshal(version)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugf("Version: %s", string(bytes))

	err = ioutil.WriteFile(outputPath, bytes, 0644)
	if err != nil {
		return nil, nil, err
	}

	k, ok := source["kubeconfig"].(string)
	if !ok {
		logger.Errorf("not a string")
		return nil, nil, errors.New("blah")
	}

	f, err := ioutil.TempFile("", "kube")
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	f.WriteString(k)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	err = f.Close()
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	clusterConfig, err := clientcmd.BuildConfigFromFlags("", f.Name())
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	clientset, err := versioned.NewForConfig(clusterConfig)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	namespace, ok := source["namespace"].(string)
	if !ok {
		logger.Errorf("namespace not a string")
		return nil, nil, errors.New("blah")
	}

	build, err := clientset.BuildV1alpha1().Builds(namespace).Get(version["build"], v1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	// Metadata consists of arbitrary name/value pairs for display in the Concourse UI,
	// and may be returned empty if not needed.
	metadata := oc.Metadata{
		{
			Name:  "gitUrl",
			Value: build.Spec.Source.Git.URL,
		},
		{
			Name:  "gitRevision",
			Value: build.Spec.Source.Git.Revision,
		},
	}

	// Here, `version` is passed through from the argument. In most cases, it makes sense
	// to retrieve the most recent version, i.e. the one in the `version` argument, and
	// then return it back unchanged. However, it is allowed to return some other version
	// or even an empty version, depending on the implementation.
	return version, metadata, nil
}

type logInfoWriter struct {
	logger *oc.Logger
}

func (l logInfoWriter) Write(p []byte) (n int, err error) {
	s := string(p)

	l.logger.Infof(strings.TrimSuffix(s, "\n"))
	return len(s), nil
}

// Out implements the ofcourse.Resource Out method, corresponding to the /opt/resource/out command.
// This is called when a Concourse job does a `put` on the resource.
func (r *Resource) Out(inputDirectory string, source oc.Source, params oc.Params,
	env oc.Environment, logger *oc.Logger) (oc.Version, oc.Metadata, error) {
	//// The `Out` function does not receive a `version` argument. Instead, we
	//// will read the version from the file created by the `In` function, assuming
	//// the pipeline does a `get` of this resource. The path to the version file
	//// must be passed in the `put` parameters.
	//versionPath, ok := params["version_path"]
	//if !ok {
	//	return nil, nil, ErrParam
	//}

	// The `inputDirectory` argument is a directory containing subdirectories for
	// all resources retrieved with `get` in a job, as well as all of the job's
	// task outputs.
	//path := fmt.Sprintf("%s/%s", inputDirectory, versionPath)
	//bytes, err := ioutil.ReadFile(path)
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//var version oc.Version
	//err = json.Unmarshal(bytes, &version)
	//if err != nil {
	//	return nil, nil, err
	//}

	clientset, k8sclient, err := getKubeconfig(logger, source)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	namespace, ok := source["namespace"].(string)
	if !ok {
		logger.Errorf("namespace not a string")
		return nil, nil, errors.New("blah")
	}
	logger.Debugf("namespace %s", namespace)

	imageName, ok := source["image"].(string)
	if !ok {
		logger.Errorf("namespace not a string")
		return nil, nil, errors.New("blah")
	}
	logger.Debugf("image %s", imageName)

	image, err := clientset.BuildV1alpha1().Images(namespace).Get(imageName, v1.GetOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}
	logger.Debugf("found: image with name: %s", image.Name)

	image = image.DeepCopy()
	image.Spec.Build.Env = append(image.Spec.Build.Env, corev1.EnvVar{
		Name:      "buildkicker",
		Value:     fmt.Sprintf("%d", time.Now().Nanosecond()),
		ValueFrom: nil,
	})
	nextBuildNumber := image.Status.BuildCounter + 1

	_, err = clientset.BuildV1alpha1().Images(namespace).Update(image)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	go func() {
		err = logs.NewBuildLogsClient(k8sclient).Tail(context.Background(), logInfoWriter{logger}, imageName, fmt.Sprintf("%d", nextBuildNumber), namespace)
		if err != nil {
			logger.Errorf(err.Error())
		}
	}()

	for {
		time.Sleep(10 * time.Second)
		image, err = clientset.BuildV1alpha1().Images(namespace).Get(imageName, v1.GetOptions{})
		if err != nil {
			logger.Errorf(err.Error())
			return nil, nil, err
		}

		if image.Status.GetCondition(v1alpha1.ConditionReady).IsTrue() {
			metadata := oc.Metadata{
				{
					Name:  "gitUrl",
					Value: image.Spec.Source.Git.URL,
				},
				{
					Name:  "gitRevision",
					Value: image.Spec.Source.Git.Revision,
				},
			}

			return oc.Version{
				"ref":   image.Status.LatestImage,
				"build": image.Status.LatestBuildRef,
			}, metadata, nil
		}
	}

	// Both `version` and `metadata` may be empty. In this case, we are returning
}

func getKubeconfig(logger *oc.Logger, source oc.Source) (*versioned.Clientset, *kubernetes.Clientset, error) {
	k, ok := source["kubeconfig"].(string)
	if !ok {
		logger.Errorf("not a string")
		return nil, nil, errors.New("blah")
	}

	f, err := ioutil.TempFile("", "kube")
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	f.WriteString(k)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	err = f.Close()
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	clusterConfig, err := clientcmd.BuildConfigFromFlags("", f.Name())
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	clientset, err := versioned.NewForConfig(clusterConfig)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	k8sClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logger.Errorf(err.Error())
		return nil, nil, err
	}

	return clientset, k8sClient, nil

}
