package session

import (
	"context"
	"net"
	"strconv"
	"time"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterLocalDomain = "svc.cluster.local"
	ResourceNamePlural = "datamoversessions"
	ResourceName       = "datamoversession"
	waitTimeout        = time.Second * 120
	waitInterval       = time.Second * 5
)

type SessionConfig struct {
	Implementation string
	Service        *corev1.Service
	SessionData    string
}

func GetServiceEndpoints(ctx context.Context, dynCli dynamic.Interface, sessionName, sessionNamespace string) (map[string]string, error) {
	sessionConfig, err := GetConfig(ctx, dynCli, sessionName, sessionNamespace)
	if err != nil {
		return nil, err
	}
	if sessionConfig.Service == nil {
		return nil, errors.New("Session config does not have a service")
	}
	hostname := sessionConfig.Service.Spec.ClusterIP
	result := map[string]string{}
	for _, portSpec := range sessionConfig.Service.Spec.Ports {
		hostPort := net.JoinHostPort(hostname, strconv.FormatInt(int64(portSpec.Port), 10))
		result[portSpec.Name] = hostPort
	}
	return result, nil
}

func GetService(ctx context.Context, dynCli dynamic.Interface, dmSession api.DatamoverSession) (*corev1.Service, error) {
	// No service is not an error
	if dmSession.Status.SessionInfo.ServiceName == "" {
		return nil, nil
	}
	serviceName := dmSession.Status.SessionInfo.ServiceName
	serviceNamespace := dmSession.Namespace
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}
	serviceData, err := dynCli.Resource(gvr).Namespace(serviceNamespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get service")
	}

	log.Log.Info("Service data object", "data", serviceData)
	var svc corev1.Service
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(serviceData.Object, &svc)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse service object")
	}
	return &svc, nil
}

// TODO: do we need a nowait version of that?
func GetConfig(ctx context.Context, dynCli dynamic.Interface, sessionName, sessionNamespace string) (*SessionConfig, error) {
	session, err := WaitForReady(ctx, func() (*api.DatamoverSession, error) { return Get(ctx, dynCli, sessionName, sessionNamespace) })
	if err != nil {
		return nil, errors.Wrap(err, "Timeout waiting for session to be ready")
	}
	service, err := GetService(ctx, dynCli, *session)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting service from session")
	}
	return &SessionConfig{
		Implementation: session.Spec.Implementation,
		Service:        service,
		SessionData:    session.Status.SessionInfo.SessionData,
	}, nil
}

func WaitForReadyByName(ctx context.Context, dynCli dynamic.Interface, sessionName, sessionNamespace string) (*api.DatamoverSession, error) {
	return WaitForReady(ctx, func() (*api.DatamoverSession, error) { return Get(ctx, dynCli, sessionName, sessionNamespace) })
}

func WaitForReady(ctx context.Context, getFunc func() (*api.DatamoverSession, error)) (*api.DatamoverSession, error) {
	return WaitForReadyWithTimeout(ctx, getFunc, waitTimeout, waitInterval)
}

func WaitForReadyWithTimeout(ctx context.Context, getFunc func() (*api.DatamoverSession, error), timeout time.Duration, interval time.Duration) (*api.DatamoverSession, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	doneWaiting := false
	for {
		session, err := getFunc()
		if err != nil {
			return nil, err
		}
		if isSessionReady(session) {
			return session, nil
		}
		if isSessionTerminated(session) {
			return session, errors.New("session terminated")
		}
		if doneWaiting {
			return session, errors.New("timeout waiting for session ti be ready")
		}
		select {
		case <-waitCtx.Done():
			doneWaiting = true
		case <-time.After(interval):
		}
	}
}

func isSessionReady(dmSession *api.DatamoverSession) bool {
	return dmSession != nil && dmSession.Status.Progress == api.ProgressReady
}

func isSessionTerminated(dmSession *api.DatamoverSession) bool {
	if dmSession != nil {
		switch dmSession.Status.Progress {
		case api.ProgressReadinessFailure, api.ProgressSessionFailure, api.ProgressValidationFailed:
			return true
		}
	}
	return false
}

func Get(ctx context.Context, dynCli dynamic.Interface, sessionName, sessionNamespace string) (*api.DatamoverSession, error) {
	client := dynCli.Resource(api.GroupVersion.WithResource(ResourceNamePlural)).Namespace(sessionNamespace)
	us, err := client.Get(ctx, sessionName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	dataMoverSession := api.DatamoverSession{}
	unstructured := us.UnstructuredContent()
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructured, &dataMoverSession)
	if err != nil {
		return nil, err
	}
	return &dataMoverSession, nil
}

func Create(ctx context.Context, dynCli dynamic.Interface, dmSession api.DatamoverSession) (*api.DatamoverSession, error) {
	dmSession.Kind = api.DatamoverSessionKind
	dmSession.APIVersion = api.GroupVersion.String()

	client := dynCli.Resource(api.GroupVersion.WithResource(ResourceNamePlural)).Namespace(dmSession.Namespace)
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&dmSession)
	if err != nil {
		return nil, err
	}
	us := &unstructured.Unstructured{}
	us.SetUnstructuredContent(data)

	res, err := client.Create(ctx, us, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	dataMoverSession := api.DatamoverSession{}
	content := res.UnstructuredContent()
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(content, &dataMoverSession)
	if err != nil {
		return nil, err
	}
	return &dataMoverSession, nil
}
