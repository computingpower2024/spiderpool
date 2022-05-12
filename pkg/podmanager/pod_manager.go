// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package podmanager

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spidernet-io/spiderpool/pkg/constant"
)

type PodManager interface {
	GetPodByName(ctx context.Context, namespace, podName string) (*corev1.Pod, error)
	GetOwnerType(ctx context.Context, pod *corev1.Pod) constant.OwnerType
	IsIPAllocatable(ctx context.Context, pod *corev1.Pod) (constant.PodStatus, bool)
	MergeAnnotations(ctx context.Context, pod *corev1.Pod, annotations map[string]string) error
	MatchLabelSelector(ctx context.Context, namespace, podName string, labelSelector *metav1.LabelSelector) (bool, error)
}

type podManager struct {
	client client.Client
}

func NewPodManager(c client.Client) PodManager {
	return &podManager{
		client: c,
	}
}

func (r *podManager) GetPodByName(ctx context.Context, namespace, podName string) (*corev1.Pod, error) {
	var pod corev1.Pod
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, &pod); err != nil {
		return nil, err
	}

	return &pod, nil
}

func (r *podManager) GetOwnerType(ctx context.Context, pod *corev1.Pod) constant.OwnerType {
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return constant.OwnerNone
	}

	var ownerType constant.OwnerType
	switch constant.OwnerType(owner.Kind) {
	case constant.OwnerDeployment:
		ownerType = constant.OwnerDeployment
	case constant.OwnerStatefuleSet:
		ownerType = constant.OwnerStatefuleSet
	case constant.OwnerDaemonSet:
		ownerType = constant.OwnerDaemonSet
	default:
		ownerType = constant.OwnerCRD
	}

	return ownerType
}

func (r *podManager) IsIPAllocatable(ctx context.Context, pod *corev1.Pod) (constant.PodStatus, bool) {
	if pod.DeletionTimestamp != nil && pod.DeletionGracePeriodSeconds != nil {
		now := time.Now()
		deletionTime := pod.DeletionTimestamp.Time
		deletionGracePeriod := time.Duration(*pod.DeletionGracePeriodSeconds) * time.Second
		if now.After(deletionTime.Add(deletionGracePeriod)) {
			return constant.PodTerminating, false
		}
	}

	if pod.Status.Phase == corev1.PodSucceeded && pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		return constant.PodSucceeded, false
	}

	if pod.Status.Phase == corev1.PodFailed && pod.Spec.RestartPolicy == corev1.RestartPolicyNever {
		return constant.PodFailed, false
	}

	if pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted" {
		return constant.PodEvicted, false
	}

	return constant.PodRunning, true
}

func (r *podManager) MergeAnnotations(ctx context.Context, pod *corev1.Pod, annotations map[string]string) error {
	merge := make(map[string]string)
	for k, v := range pod.Annotations {
		merge[k] = v
	}

	for k, v := range annotations {
		merge[k] = v
	}

	pod.Annotations = merge
	if err := r.client.Update(ctx, pod); err != nil {
		return err
	}

	return nil
}

func (r *podManager) MatchLabelSelector(ctx context.Context, namespace, podName string, labelSelector *metav1.LabelSelector) (bool, error) {
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return false, err
	}

	var pods corev1.PodList
	err = r.client.List(
		ctx,
		&pods,
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: selector},
		client.MatchingFields{".metadata.name": podName},
	)
	if err != nil {
		return false, err
	}

	if len(pods.Items) == 0 {
		return false, nil
	}

	return true, nil
}
