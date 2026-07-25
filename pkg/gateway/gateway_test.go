package gateway

import (
	"testing"

	"github.com/adrianliechti/loop/pkg/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func parentStatus(namespace, name string, accepted metav1.ConditionStatus) gatewayv1.RouteParentStatus {
	ref := gatewayv1.ParentReference{
		Name: gatewayv1.ObjectName(name),
	}

	if namespace != "" {
		ref.Namespace = kubernetes.Ptr(gatewayv1.Namespace(namespace))
	}

	return gatewayv1.RouteParentStatus{
		ParentRef: ref,

		Conditions: []metav1.Condition{
			{
				Type:   string(gatewayv1.RouteConditionAccepted),
				Status: accepted,
			},
		},
	}
}

func httpRoute(namespace string, parents ...gatewayv1.RouteParentStatus) gatewayv1.HTTPRoute {
	return gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},

		Status: gatewayv1.HTTPRouteStatus{
			RouteStatus: gatewayv1.RouteStatus{
				Parents: parents,
			},
		},
	}
}

func TestParentAccepted(t *testing.T) {
	tests := []struct {
		name string

		route     gatewayv1.HTTPRoute
		namespace string
		gateway   string

		want bool
	}{
		{
			name:      "no status at all is tolerated",
			route:     httpRoute("app"),
			namespace: "app",
			gateway:   "public",
			want:      true,
		},
		{
			name:      "accepted",
			route:     httpRoute("app", parentStatus("", "public", metav1.ConditionTrue)),
			namespace: "app",
			gateway:   "public",
			want:      true,
		},
		{
			name:      "rejected",
			route:     httpRoute("app", parentStatus("", "public", metav1.ConditionFalse)),
			namespace: "app",
			gateway:   "public",
			want:      false,
		},
		{
			// The gateway a cross-namespace route names only admits it if the
			// listener's allowedRoutes says so; the refusal lands here.
			name:      "rejected across namespaces",
			route:     httpRoute("app", parentStatus("infra", "public", metav1.ConditionFalse)),
			namespace: "infra",
			gateway:   "public",
			want:      false,
		},
		{
			name:      "accepted across namespaces",
			route:     httpRoute("app", parentStatus("infra", "public", metav1.ConditionTrue)),
			namespace: "infra",
			gateway:   "public",
			want:      true,
		},
		{
			// An unset namespace in the status ref means the route's own, so
			// this entry describes app/public and says nothing about infra/public.
			name:      "status for the same name in another namespace is not ours",
			route:     httpRoute("app", parentStatus("", "public", metav1.ConditionFalse)),
			namespace: "infra",
			gateway:   "public",
			want:      true,
		},
		{
			name:      "status for a different gateway is ignored",
			route:     httpRoute("app", parentStatus("", "other", metav1.ConditionFalse)),
			namespace: "app",
			gateway:   "public",
			want:      true,
		},
		{
			// A route may name the same gateway over several sections; one
			// listener taking it is enough.
			name: "one accepting listener is enough",
			route: httpRoute("app",
				parentStatus("", "public", metav1.ConditionFalse),
				parentStatus("", "public", metav1.ConditionTrue),
			),
			namespace: "app",
			gateway:   "public",
			want:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parentAccepted(test.route, test.namespace, test.gateway); got != test.want {
				t.Fatalf("parentAccepted = %v, want %v", got, test.want)
			}
		})
	}
}

func gateway(addresses ...gatewayv1.GatewayStatusAddress) gatewayv1.Gateway {
	return gatewayv1.Gateway{
		Status: gatewayv1.GatewayStatus{
			Addresses: addresses,
		},
	}
}

func address(t *gatewayv1.AddressType, value string) gatewayv1.GatewayStatusAddress {
	return gatewayv1.GatewayStatusAddress{
		Type:  t,
		Value: value,
	}
}

func TestGatewayAddress(t *testing.T) {
	ipType := kubernetes.Ptr(gatewayv1.IPAddressType)
	hostnameType := kubernetes.Ptr(gatewayv1.HostnameAddressType)

	tests := []struct {
		name    string
		gateway gatewayv1.Gateway
		want    string
	}{
		{
			name:    "no addresses",
			gateway: gateway(),
			want:    "",
		},
		{
			name:    "ip preferred over hostname",
			gateway: gateway(address(ipType, "10.0.0.1"), address(hostnameType, "lb.example.com")),
			want:    "10.0.0.1",
		},
		{
			name:    "hostname only",
			gateway: gateway(address(hostnameType, "lb.example.com")),
			want:    "lb.example.com",
		},
		{
			// Type is optional and defaults to IPAddress, so an unset one must
			// not lose out to an explicit hostname.
			name:    "unset type counts as ip",
			gateway: gateway(address(hostnameType, "lb.example.com"), address(nil, "10.0.0.1")),
			want:    "10.0.0.1",
		},
		{
			name:    "unset type alone",
			gateway: gateway(address(nil, "10.0.0.1")),
			want:    "10.0.0.1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gatewayAddress(test.gateway); got != test.want {
				t.Fatalf("gatewayAddress = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInScope(t *testing.T) {
	unscoped := &Gateway{}

	if !unscoped.inScope("anything") {
		t.Fatal("no namespaces configured should mean everything is in scope")
	}

	scoped := &Gateway{
		options: GatewayOptions{
			Namespaces: []string{"app", "shop"},
		},
	}

	if !scoped.inScope("app") {
		t.Fatal("app should be in scope")
	}

	if scoped.inScope("infra") {
		t.Fatal("infra should not be in scope")
	}
}
