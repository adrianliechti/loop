package kubernetes

import (
	"context"
	"io"
	"net/http"
	"os"
	"regexp"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
)

func (c *client) Apply(ctx context.Context, namespace string, reader io.Reader) error {
	if namespace == "" {
		namespace = c.Namespace()
	}

	data, err := io.ReadAll(reader)

	if err != nil {
		return err
	}

	docs, err := splitDocuments(data)

	if err != nil {
		return err
	}

	discovery, err := discovery.NewDiscoveryClientForConfig(c.Config())

	if err != nil {
		return err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discovery))

	serializer := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	for _, doc := range docs {
		obj := &unstructured.Unstructured{}
		_, gvk, err := serializer.Decode(doc, nil, obj)

		if err != nil {
			if runtime.IsMissingKind(err) {
				continue
			}

			return err
		}

		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)

		if err != nil {
			return err
		}

		if obj.GetNamespace() == "" && mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			obj.SetNamespace(namespace)
		}

		i := c.Resource(mapping.Resource).Namespace(obj.GetNamespace())

		if _, err := i.Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{
			Force:        true,
			FieldManager: "platform",
		}); err != nil {
			return err
		}
	}

	return nil
}

func (c *client) ApplyFile(ctx context.Context, namespace string, path string) error {
	f, err := os.Open(path)

	if err != nil {
		return err
	}

	defer f.Close()

	return c.Apply(ctx, namespace, f)
}

func (c *client) ApplyURL(ctx context.Context, namespace string, url string) error {
	resp, err := http.Get(url)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return c.Apply(ctx, namespace, resp.Body)
}

func splitDocuments(data []byte) ([][]byte, error) {
	re := regexp.MustCompile(`(?m)^---$`)

	var results [][]byte

	for _, doc := range re.Split(string(data), -1) {
		results = append(results, []byte(doc))
	}

	return results, nil
}
