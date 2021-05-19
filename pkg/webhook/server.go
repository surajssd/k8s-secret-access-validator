// Package webhook contains code for admission webhook.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	v1 "k8s.io/api/admission/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// ValidateSecretAccess is a service to mutate default service account.
func ValidateSecretAccess(w http.ResponseWriter, r *http.Request) {
	response := serve(w, r)

	respBytes, err := json.Marshal(response)
	if err != nil {
		klog.Errorf("marshaling JSON data: %v", err)
	}

	klog.Infof("Sending Response: %s", string(respBytes))

	if _, err := w.Write(respBytes); err != nil {
		klog.Errorf("writing response data: %v", err)
	}
}

func returnError(err error) *v1.AdmissionResponse {
	klog.Error(err)
	return &v1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func serve(w http.ResponseWriter, r *http.Request) v1.AdmissionReview {
	requestedAdmissionReview := v1.AdmissionReview{}
	response := v1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
	}

	deserializer := scheme.Codecs.UniversalDeserializer()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		err = fmt.Errorf("reading request body: %w", err)
		response.Response = returnError(err)
		return response
	}

	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		err = fmt.Errorf("decoding request: %w", err)
		response.Response = returnError(err)
		return response
	}

	resp, err := validateSecretAccess(requestedAdmissionReview)
	if err != nil {
		klog.Error(fmt.Errorf("validating secret: %w", err))
		resp.Result = &metav1.Status{
			Message: err.Error(),
		}
		response.Response = resp
		return response
	}

	response.Response = resp
	return response
}

func getSecretsFromPodSpec(podSpec corev1.PodSpec) []string {
	var ret []string

	// Get all the volumes that try to access secret.
	for _, vol := range podSpec.Volumes {
		if vol.Secret != nil {
			ret = append(ret, vol.Secret.SecretName)
		}
	}

	// Get all the secrets accessed via environment variables.
	for _, cnt := range podSpec.Containers {
		for _, env := range cnt.EnvFrom {
			if env.SecretRef != nil {
				ret = append(ret, env.SecretRef.Name)
			}
		}

		for _, env := range cnt.Env {
			if env.ValueFrom == nil {
				continue
			}

			if env.ValueFrom.SecretKeyRef == nil {
				continue
			}

			ret = append(ret, env.ValueFrom.SecretKeyRef.Name)
		}
	}

	return ret
}

func getSecret(req *v1.AdmissionRequest) ([]string, error) {
	// TODO: Remove all the supported objects are covered.
	klog.Infof("Got this object of Kind: %#v.", req.Kind)

	if req.Kind.Kind == "Pod" && req.Kind.Version == "v1" {
		klog.Info("Received Pod.")

		pod := &corev1.Pod{}
		if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
			return nil, fmt.Errorf("unmarshalling request object of type: %q: %w", req.Kind, err)
		}

		return getSecretsFromPodSpec(pod.Spec), nil
	}

	return nil, fmt.Errorf("unsupported kind received: %#v", req.Kind)
}

func isControllerUser(u string) bool {
	users := map[string]interface{}{
		"system:serviceaccount:kube-system:cronjob-controller":     nil,
		"system:serviceaccount:kube-system:daemon-set-controller":  nil,
		"system:serviceaccount:kube-system:deployment-controller":  nil,
		"system:serviceaccount:kube-system:job-controller":         nil,
		"system:serviceaccount:kube-system:replicaset-controller":  nil,
		"system:serviceaccount:kube-system:replication-controller": nil,
		"system:serviceaccount:kube-system:statefulset-controller": nil,
	}

	if _, ok := users[u]; ok {
		return true
	}

	return false
}

func validateSecretAccess(r v1.AdmissionReview) (*v1.AdmissionResponse, error) {
	req := r.Request
	response := &v1.AdmissionResponse{
		UID: req.UID,
	}
	user := req.UserInfo

	// If the user is of controller type like replicaset, job, etc then ignore.
	if isControllerUser(user.Username) {
		response.Allowed = true
		return response, nil
	}

	// Talk to the APIServer
	config, err := rest.InClusterConfig()
	if err != nil {
		return response, fmt.Errorf("reading incluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return response, fmt.Errorf("creating clientset: %w", err)
	}

	// TODO: We should also check if the group this user belongs to has permissions.
	auth := clientset.AuthorizationV1().SubjectAccessReviews()

	secrets, err := getSecret(req)
	if err != nil {
		return response, fmt.Errorf("getting secrets: %w", err)
	}

	for _, secret := range secrets {
		sar := &authzv1.SubjectAccessReview{
			Spec: authzv1.SubjectAccessReviewSpec{
				ResourceAttributes: &authzv1.ResourceAttributes{
					Verb:      "get",
					Resource:  "secrets",
					Name:      secret,
					Namespace: req.Namespace,
				},
				User: user.Username,
			},
		}

		authResp, err := auth.Create(context.TODO(), sar, metav1.CreateOptions{})
		if err != nil {
			return response, fmt.Errorf("checking permissions: %w", err)
		}

		// According to the value of this set the Allowed to appropriate value.
		if !authResp.Status.Allowed {
			return response, fmt.Errorf("User %q does not have access to the secret %q in the namespace %q.", user.Username, secret, req.Namespace)
		}
	}

	response.Allowed = true
	return response, nil
}
