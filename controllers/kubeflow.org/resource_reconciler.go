package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"text/template"

	profilev2alpha1 "github.com/pluralsh/kubeflow-profile-controller/apis/kubeflow.org/v2alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/yaml"
)

func deserializeExtraResources(profile *profilev2alpha1.Profile) ([]*unstructured.Unstructured, error) {
	var out bytes.Buffer
	for ind, extra := range profile.Spec.ExtraResources {
		tmpl, err := template.ParseFiles(extra.FileName)
		if err != nil {
			return nil, err
		}

		if ind > 0 {
			out.Write([]byte("---\n"))
		}

		varsJSON, err := extra.Vars.MarshalJSON()
		if err != nil {
			return nil, err
		}

		var tempVars interface{}
		if varErr := json.Unmarshal(varsJSON, &tempVars); varErr != nil {
			return nil, varErr
		}

		varsMap := tempVars.(map[string]interface{})
		varsMap["Namespace"] = profile.Name

		if err := tmpl.Execute(&out, varsMap); err != nil {
			return nil, err
		}
	}
	return splitYaml(out.Bytes())
}

func (r *ProfileReconciler) reconcileExtraResources(ctx context.Context, profile *profilev2alpha1.Profile) error {
	logger := r.Log.WithValues("profile", profile.Name)
	objs, err := deserializeExtraResources(profile)
	if err != nil {
		return err
	}

	for _, obj := range objs {
		obj.SetNamespace(profile.Name)

		if err := controllerutil.SetControllerReference(profile, obj, r.Scheme); err != nil {
			return err
		}

		logger.Info("Applying extra resource", "kind", obj.GetKind(), "name", obj.GetName())
		if err := r.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner("profile-controller")); err != nil {
			return err
		}
	}

	return nil
}

func splitYaml(yamlData []byte) ([]*unstructured.Unstructured, error) {
	d := kubeyaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)
	var objs []*unstructured.Unstructured
	for {
		ext := runtime.RawExtension{}
		if err := d.Decode(&ext); err != nil {
			if err == io.EOF {
				break
			}
			return objs, fmt.Errorf("failed to unmarshal manifest: %v", err)
		}
		ext.Raw = bytes.TrimSpace(ext.Raw)
		if len(ext.Raw) == 0 || bytes.Equal(ext.Raw, []byte("null")) {
			continue
		}
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(ext.Raw, u); err != nil {
			return objs, fmt.Errorf("failed to unmarshal manifest: %v", err)
		}
		objs = append(objs, u)
	}
	return objs, nil
}
