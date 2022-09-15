/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composed"
	ucomposite "github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	v1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/crossplane/crossplane/internal/controller/apiextensions/composite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8s_json "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/json"

	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

func unmarshal(data string, v runtime.Object) error {
	_, _, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode([]byte(data), nil, v)
	return err
}

// can we reuse an existing struct?
type Composition struct {
	metav1.ObjectMeta
	metav1.TypeMeta

	Spec struct {
		Resources []v1.ComposedTemplate `json:"resources"`
	} `json:"spec"`
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (m *Composition) DeepCopyObject() runtime.Object {
	out := &Composition{}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}

func eval(compositeResource, composition string) (string, error) {
	cpr := ucomposite.Unstructured{}
	if err := unmarshal(compositeResource, &cpr); err != nil {
		return "", fmt.Errorf("cannot unmarshal composite resource: %w", err)
	}

	comp := Composition{}
	if err := unmarshal(composition, &comp); err != nil {
		return "", fmt.Errorf("cannot unmarshal composition: %w", err)
	}

	// TODO: allow bootstrapped objects in input

	// TODO: also an array of mutations on them (to emulate changes done by the provider, for example status updates)
	cd := &composed.Unstructured{
		Unstructured: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "database.gcp.crossplane.io/v1beta1",
				"kind":       "CloudSQLInstance",
				"labels": map[string]interface{}{
					"crossplane.io/composite": "foo",
				},
				"metadata": &metav1.ObjectMeta{Name: "cd"},
			},
		},
	}

	client := &test.MockClient{MockCreate: test.NewMockCreateFn(nil)}
	r := composite.NewAPIDryRunRenderer(client)
	err := r.Render(context.Background(), &cpr, cd, comp.Spec.Resources[0])
	if err != nil {
		return "", fmt.Errorf("cannot render composition: %w", err)
	}

	obj, _ := ObjToYaml(cd)
	return string(obj), nil
}

func main() {
	serve()
}

func ObjToYaml(obj runtime.Object) ([]byte, error) {
	e := k8s_json.NewSerializerWithOptions(k8s_json.DefaultMetaFactory, nil, nil, k8s_json.SerializerOptions{Yaml: true, Pretty: true, Strict: true})
	buf := &bytes.Buffer{}
	buf.WriteString("---\n")
	if err := e.Encode(obj, buf); err != nil {
		fmt.Println("error: ", err)
		return []byte{}, err
	}

	replacedBuff := bytes.ReplaceAll(buf.Bytes(), []byte("status: {}\n"), []byte{})
	replacedBuff = bytes.ReplaceAll(replacedBuff, []byte("\nspec: {}\n"), []byte("\n"))
	replacedBuff = bytes.ReplaceAll(replacedBuff, []byte("  creationTimestamp: null\n"), []byte{})
	return replacedBuff, nil
}
