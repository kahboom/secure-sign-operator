package utils

import (
	"errors"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateDeployment(instance *v1alpha1.Fulcio, deploymentName string, sa string, labels map[string]string) (*appsv1.Deployment, error) {
	if instance.Status.ServerConfigRef == nil {
		return nil, errors.New("server config ref is not specified")
	}
	if instance.Status.Certificate.PrivateKeyRef == nil {
		return nil, errors.New("private key secret is not specified")
	}

	if instance.Status.Certificate.CARef == nil {
		return nil, errors.New("CA secret is not specified")
	}

	replicas := int32(1)
	mode := int32(0666)
	args := []string{
		"serve",
		"--port=5555",
		"--grpc-port=5554",
		"--ca=fileca",
		"--fileca-key",
		"/var/run/fulcio-secrets/key.pem",
		"--fileca-cert",
		"/var/run/fulcio-secrets/cert.pem",
		fmt.Sprintf("--ct-log-url=http://ctlog.%s.svc/trusted-artifact-signer", instance.Namespace)}

	env := make([]corev1.EnvVar, 0)
	if instance.Status.Certificate.PrivateKeyPasswordRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: instance.Status.Certificate.PrivateKeyPasswordRef.Key,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instance.Status.Certificate.PrivateKeyPasswordRef.Name,
					},
				},
			},
		})
		args = append(args, "--fileca-key-passwd", "$(PASSWORD)")
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					Containers: []corev1.Container{
						{
							Name:  "fulcio-server",
							Image: constants.FulcioServerImage,
							Args:  args,
							Env:   env,
							Ports: []corev1.ContainerPort{
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5555,
								},
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5554,
								},
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 2112,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(5555),
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(5555),
									},
								},
								// we need to specify all defaults https://github.com/stolostron/multicluster-observability-operator/pull/301
								InitialDelaySeconds: 10,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "fulcio-config",
									MountPath: "/etc/fulcio-config",
								},
								{
									Name:      "oidc-info",
									MountPath: "/var/run/fulcio",
								},
								{
									Name:      "fulcio-cert",
									MountPath: "/var/run/fulcio-secrets",
									ReadOnly:  true,
								},
							},
						},
					},
					AutomountServiceAccountToken: &[]bool{true}[0],
					Volumes: []corev1.Volume{
						{
							Name: "fulcio-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: *instance.Status.ServerConfigRef,
								},
							},
						},
						{
							Name: "oidc-info",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ConfigMap: &corev1.ConfigMapProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "kube-root-ca.crt",
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "ca.crt",
														Path: "ca.crt",
														Mode: &mode,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Name: "fulcio-cert",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: instance.Status.Certificate.PrivateKeyRef.Name,
												},
												Items: []corev1.KeyToPath{
													{
														Key:  instance.Status.Certificate.PrivateKeyRef.Key,
														Path: "key.pem",
													},
												},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: instance.Status.Certificate.CARef.Name,
												},
												Items: []corev1.KeyToPath{
													{
														Key:  instance.Status.Certificate.CARef.Key,
														Path: "cert.pem",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}, nil
}
