package utils

import (
	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// IsModuleDifferent compares two provided Module
func IsModuleDifferent(actual, new *corev1beta1.Module) bool {
	if actual.Spec.Flavor == new.Spec.Flavor && actual.Spec.Image == new.Spec.Image && actual.Spec.Replicas == new.Spec.Replicas && EqualEnvVars(actual.Spec.Env, new.Spec.Env) {
		return false
	}
	return true
}

// EqualEnvVars compares two provided EnvVar
func EqualEnvVars(oldVars, newVars []corev1.EnvVar) bool {
	if len(oldVars) != len(newVars) {
		return false
	}

	for _, oldVar := range oldVars {
		for _, newVar := range newVars {
			if oldVar.Name == newVar.Name && oldVar.Value != newVar.Value {
				return false
			}
		}
	}

	return true
}
