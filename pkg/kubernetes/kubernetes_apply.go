package kubernetes

import (
	"context"
	"fmt"
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

var documentSeparator = regexp.MustCompile(`(?m)^---$`)

func (c *client) Apply(ctx context.Context, namespace string, reader io.Reader) error {
	if namespace == "" {
		namespace = c.Namespace()
	}

	data, err := io.ReadAll(reader)

	if err != nil {
		return err
	}

	docs := splitDocuments(data)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status %s fetching %s", resp.Status, url)
	}

	return c.Apply(ctx, namespace, resp.Body)
}

func splitDocuments(data []byte) [][]byte {
	parts := documentSeparator.Split(string(data), -1)

	results := make([][]byte, len(parts))

	for i, doc := range parts {
		results[i] = []byte(doc)
	}

	return results
}
