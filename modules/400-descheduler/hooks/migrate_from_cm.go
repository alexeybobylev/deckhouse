/*
Copyright 2022 Flant JSC

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

package hooks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	"github.com/flant/addon-operator/sdk"
	"github.com/flant/shell-operator/pkg/kube/object_patch"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/deckhouse/go_lib/dependency"
)

var _ = sdk.RegisterFunc(&go_hook.HookConfig{
	OnBeforeHelm: &go_hook.OrderedConfig{Order: 1},
}, dependency.WithExternalDependencies(createFirstDeschedulerCR))

func createFirstDeschedulerCR(input *go_hook.HookInput, dc dependency.Container) error {
	kubeCl, err := dc.GetK8sClient()
	if err != nil {
		return fmt.Errorf("cannot init Kubernetes client: %v", err)
	}

	cm, err := kubeCl.CoreV1().ConfigMaps("d8-system").Get(context.TODO(), "descheduler-config-migration", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		input.LogEntry.Infof("nothing to migrate: %s", err)
		return nil
	}
	if err != nil {
		return err
	}

	configJSON, ok := cm.Data["config"]
	if !ok {
		return fmt.Errorf("failed to get \"config\" field from CM \"descheduler-config-migration\"")
	}

	if !gjson.Valid(configJSON) {
		return fmt.Errorf("config json is invalid: %s", configJSON)
	}
	config := gjson.Parse(configJSON)

	var deschedulerCr []byte
	deschedulerCr, err = sjson.SetBytes(deschedulerCr, "apiVersion", "deckhouse.io/v1alpha1")
	if err != nil {
		return err
	}
	deschedulerCr, err = sjson.SetBytes(deschedulerCr, "kind", "Descheduler")
	if err != nil {
		return err
	}
	deschedulerCr, err = sjson.SetBytes(deschedulerCr, "metadata.name", "legacy")
	if err != nil {
		return err
	}

	if value := config.Get("removeDuplicates"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removeDuplicates.enabled", true)
		if err != nil {
			return err
		}
	}
	if value := config.Get("lowNodeUtilization"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.lowNodeUtilization.enabled", true)
		if err != nil {
			return err
		}
	}
	if value := config.Get("highNodeUtilization"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.highNodeUtilization.enabled", true)
		if err != nil {
			return err
		}
	}
	if value := config.Get("removePodsViolatingInterPodAntiAffinity"); value.Exists() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsViolatingInterPodAntiAffinity.enabled", value.Bool())
	} else {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsViolatingInterPodAntiAffinity.enabled", true)
	}
	if err != nil {
		return err
	}
	if value := config.Get("removePodsViolatingNodeAffinity"); value.Exists() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsViolatingNodeAffinity.enabled", value.Bool())
	} else {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsViolatingNodeAffinity.enabled", true)
	}
	if err != nil {
		return err
	}
	if value := config.Get("removePodsViolatingNodeTaints"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsViolatingNodeTaints.enabled", true)
		if err != nil {
			return err
		}
	}
	if value := config.Get("removePodsViolatingTopologySpreadConstraint"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsViolatingTopologySpreadConstraint.enabled", true)
		if err != nil {
			return err
		}
	}
	if value := config.Get("removePodsHavingTooManyRestarts"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.removePodsHavingTooManyRestarts.enabled", true)
		if err != nil {
			return err
		}
	}
	if value := config.Get("podLifeTime"); value.Exists() && value.Bool() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deschedulerPolicy.strategies.podLifeTime.enabled", true)
		if err != nil {
			return err
		}
	}

	if value := config.Get("nodeSelector"); value.Exists() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deploymentTemplate.nodeSelector", value.Value())
		if err != nil {
			return err
		}
	}

	if value := config.Get("tolerations"); value.Exists() {
		deschedulerCr, err = sjson.SetBytes(deschedulerCr, "spec.deploymentTemplate.tolerations", value.Value())
		if err != nil {
			return err
		}
	}

	var object interface{}
	err = json.Unmarshal(deschedulerCr, &object)
	if err != nil {
		return err
	}

	input.PatchCollector.Create(object, object_patch.IgnoreIfExists())
	input.PatchCollector.Delete("v1", "ConfigMap", "d8-system", "descheduler-config-migration")

	return nil
}
