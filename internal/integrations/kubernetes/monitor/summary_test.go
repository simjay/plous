package monitor

import (
	"encoding/json"
	"strings"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func summaryFixture(t *testing.T, data string) unstructured.Unstructured {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["kind"]; !exists {
		fields["kind"] = json.RawMessage(`"Fixture"`)
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var object unstructured.Unstructured
	if err := object.UnmarshalJSON(encoded); err != nil {
		t.Fatal(err)
	}
	return object
}

func findSummarySection(t *testing.T, sections []DetailSection, title string) DetailSection {
	t.Helper()
	for _, section := range sections {
		if section.Title == title {
			return section
		}
	}
	t.Fatalf("section %q missing: %#v", title, sections)
	return DetailSection{}
}

func findSummaryField(t *testing.T, section DetailSection, label string) string {
	t.Helper()
	for _, field := range section.Fields {
		if field.Label == label {
			return field.Value
		}
	}
	t.Fatalf("field %q missing from %#v", label, section)
	return ""
}

func TestPodSummaryJoinsContainerStatesByName(t *testing.T) {
	pod := summaryFixture(t, `{
	  "apiVersion":"v1", "kind":"Pod", "metadata":{"name":"checkout","namespace":"commerce","uid":"pod-1","creationTimestamp":"2026-09-29T12:00:00Z","ownerReferences":[{"kind":"ReplicaSet","name":"checkout-rs","uid":"rs-1","apiVersion":"apps/v1"}]},
	  "spec":{"nodeName":"worker-a","containers":[
	    {"name":"app","image":"checkout:v2","resources":{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"memory":"256Mi"}},"readinessProbe":{"httpGet":{"path":"/ready","port":8080},"periodSeconds":5,"timeoutSeconds":2}},
	    {"name":"worker","image":"worker:v1"}
	  ],"initContainers":[{"name":"migrate","image":"migrate:v1"},{"name":"proxy","image":"proxy:v1","restartPolicy":"Always"}],"ephemeralContainers":[{"name":"debug","image":"busybox"}],"volumes":[{"name":"config","configMap":{"name":"checkout-config"}}]},
	  "status":{"phase":"Running","podIP":"10.1.2.3","containerStatuses":[
	    {"name":"worker","ready":true,"restartCount":0,"state":{"running":{"startedAt":"2026-09-29T12:00:01Z"}}},
	    {"name":"app","ready":false,"restartCount":12,"state":{"waiting":{"reason":"CrashLoopBackOff","message":"Back-off restarting failed container"}},"lastState":{"terminated":{"exitCode":137,"reason":"OOMKilled"}}}
	  ],"initContainerStatuses":[
	    {"name":"proxy","ready":true,"started":true,"restartCount":1,"state":{"running":{}}},
	    {"name":"migrate","ready":false,"restartCount":0,"state":{"terminated":{"exitCode":0,"reason":"Completed"}}}
	  ],"ephemeralContainerStatuses":[{"name":"debug","ready":false,"restartCount":0,"state":{"running":{}}}],"conditions":[{"type":"Ready","status":"False","reason":"ContainersNotReady","message":"app is not ready"}]}
	}`)
	sections := Summary(kubecommon.Resource{Name: "pods", Kind: "Pod", Namespaced: true}, pod)
	overview := findSummarySection(t, sections, "Pod overview")
	for label, want := range map[string]string{"Status": "CrashLoopBackOff", "Ready": "2/3", "Restarts": "13", "Namespace": "commerce", "Owner": "ReplicaSet/checkout-rs", "Node": "worker-a", "Pod IP": "10.1.2.3"} {
		if got := findSummaryField(t, overview, label); got != want {
			t.Errorf("%s=%q, want %q", label, got, want)
		}
	}
	containers := findSummarySection(t, sections, "Containers")
	if len(containers.Rows) != 5 {
		t.Fatalf("containers=%#v", containers.Rows)
	}
	for i, want := range []string{
		"app|app|false|CrashLoopBackOff|12|OOMKilled (exit 137)",
		"worker|app|true|Running|0|none",
		"migrate|init|completed|Completed (exit 0)|0|none",
		"proxy|sidecar|true|Running|1|none",
		"debug|ephemeral|false|Running|0|none",
	} {
		if got := strings.Join(containers.Rows[i], "|"); got != want {
			t.Errorf("row %d=%q, want %q", i, got, want)
		}
	}
	app := findSummarySection(t, sections, "Container · app")
	if got := findSummaryField(t, app, "Requests"); got != "cpu=100m, memory=128Mi" {
		t.Errorf("requests=%q", got)
	}
	if got := findSummaryField(t, app, "Readiness"); !strings.Contains(got, "HTTP Pod IP:8080/ready") || !strings.Contains(got, "every 5s") || !strings.Contains(got, "timeout 2s") {
		t.Errorf("probe=%q", got)
	}
	text := summaryText(kubecommon.Resource{Name: "pods", Kind: "Pod"}, pod)
	for _, raw := range []string{"map[", `"httpGet":`, `"terminated":`, "lastState:"} {
		if strings.Contains(text, raw) {
			t.Errorf("raw structure in summary: %q", raw)
		}
	}
	if strings.Index(text, "ContainersNotReady") > strings.Index(text, "UID: pod-1") {
		t.Error("metadata displaced operational conditions")
	}
}

func TestPodSummaryShowsInitFailureAndUnreportedContainer(t *testing.T) {
	pod := summaryFixture(t, `{"kind":"Pod","spec":{"containers":[{"name":"app"}],"initContainers":[{"name":"migrate"}]},"status":{"phase":"Pending","initContainerStatuses":[{"name":"migrate","restartCount":3,"state":{"terminated":{"reason":"Error","exitCode":2}}}]}}`)
	sections := Summary(kubecommon.Resource{Name: "pods", Kind: "Pod"}, pod)
	if got := findSummaryField(t, sections[0], "Status"); got != "Init:Error" {
		t.Fatal(got)
	}
	containers := findSummarySection(t, sections, "Containers")
	if containers.Rows[0][3] != "Not reported" || containers.Rows[1][3] != "Error (exit 2)" {
		t.Fatalf("states=%#v", containers.Rows)
	}
}

func TestConfigSummariesRenderDataAndMaskSecretValues(t *testing.T) {
	config := summaryFixture(t, `{"apiVersion":"v1","kind":"ConfigMap","data":{"app.conf":"timeout=30\nretries=2","empty":""},"binaryData":{"logo":"AQID"}}`)
	data := findSummarySection(t, Summary(kubecommon.Resource{Name: "configmaps", Kind: "ConfigMap"}, config), "Data")
	if len(data.Rows) != 3 || data.Rows[0][1] != "timeout=30\nretries=2" || data.Rows[1][1] != "" || data.Rows[2][1] != "<binary, 3 bytes>" {
		t.Fatalf("config data=%#v", data)
	}
	secret := summaryFixture(t, `{"apiVersion":"v1","kind":"Secret","type":"Opaque","metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"do-not-show"}},"data":{"token":"c2VjcmV0"},"stringData":{"password":"secret-value"}}`)
	text := summaryText(kubecommon.Resource{Name: "secrets", Kind: "Secret"}, secret)
	for _, value := range []string{"c2VjcmV0", "secret-value", "do-not-show"} {
		if strings.Contains(text, value) {
			t.Fatalf("secret value leaked: %s", value)
		}
	}
	if !strings.Contains(text, "token · <hidden>") || !strings.Contains(text, "password · <hidden>") {
		t.Fatal(text)
	}
}

func TestSummaryUsesNativeAPIIdentityAndGenericStatusWithoutGuessedHealth(t *testing.T) {
	object := summaryFixture(t, `{"apiVersion":"example.io/v1","kind":"Pod","metadata":{"name":"other"},"spec":{"containers":[{"name":"not-a-container"}]},"status":{"phase":"Reconciling","replicas":4,"nested":{"private":"not-a-summary"},"conditions":[{"type":"Synced","status":"Unknown"}]}}`)
	r := kubecommon.Resource{Group: "example.io", Version: "v1", Name: "pods", Kind: "Pod"}
	text := summaryText(r, object)
	if strings.Contains(text, "Containers") || strings.Contains(text, "not-a-container") || strings.Contains(text, "not-a-summary") || strings.Contains(text, "Healthy") {
		t.Fatal(text)
	}
	for _, expected := range []string{"Reconciling", "replicas · 4", "Synced · Unknown", "Open YAML", "example.io/v1"} {
		if !strings.Contains(text, expected) {
			t.Errorf("missing %q: %s", expected, text)
		}
	}
}

func TestNativeSummaryResourceFields(t *testing.T) {
	for _, tc := range []struct {
		group, name, kind, object string
		want                      []string
	}{
		{"apps", "deployments", "Deployment", `{"spec":{"replicas":4,"strategy":{"type":"RollingUpdate"},"selector":{"matchLabels":{"app":"checkout"}},"template":{"spec":{"containers":[{"name":"app","image":"checkout:v2"}]}}},"status":{"readyReplicas":3,"updatedReplicas":2,"availableReplicas":3}}`, []string{"Desired replicas: 4", "Ready replicas: 3", "Updated replicas: 2", "Strategy: RollingUpdate", "Pod selector: app=checkout", "checkout:v2"}},
		{"apps", "statefulsets", "StatefulSet", `{"spec":{"replicas":3,"serviceName":"db","podManagementPolicy":"OrderedReady"},"status":{"currentRevision":"db-old","updateRevision":"db-new","readyReplicas":2}}`, []string{"Service: db", "Pod management: OrderedReady", "Current revision: db-old", "Update revision: db-new"}},
		{"apps", "daemonsets", "DaemonSet", `{"status":{"desiredNumberScheduled":5,"numberReady":4,"numberMisscheduled":1}}`, []string{"Desired scheduled: 5", "Ready: 4", "Misscheduled: 1"}},
		{"apps", "replicasets", "ReplicaSet", `{"spec":{"replicas":2},"status":{"replicas":2,"readyReplicas":1}}`, []string{"Desired replicas: 2", "Current replicas: 2", "Ready replicas: 1"}},
		{"", "replicationcontrollers", "ReplicationController", `{"spec":{"replicas":2,"selector":{"app":"legacy"}},"status":{"readyReplicas":1}}`, []string{"Desired replicas: 2", "Pod selector: app=legacy"}},
		{"batch", "jobs", "Job", `{"spec":{"completions":4,"parallelism":2,"backoffLimit":3},"status":{"active":1,"succeeded":2,"failed":1}}`, []string{"Completions: 4", "Parallelism: 2", "Backoff limit: 3", "Succeeded: 2", "Failed: 1"}},
		{"batch", "cronjobs", "CronJob", `{"spec":{"schedule":"0 * * * *","timeZone":"Etc/UTC","suspend":false,"concurrencyPolicy":"Forbid"},"status":{"active":[{"kind":"Job","name":"backup-1","namespace":"ops"}]}}`, []string{"Schedule: 0 * * * *", "Time zone: Etc/UTC", "Suspended: false", "Concurrency policy: Forbid", "Active jobs: 1", "backup-1"}},
		{"", "services", "Service", `{"spec":{"type":"NodePort","clusterIP":"10.0.0.1","selector":{"app":"web"},"ports":[{"name":"http","port":80,"targetPort":"http","protocol":"TCP","nodePort":30080}]}}`, []string{"Type: NodePort", "Cluster IP: 10.0.0.1", "Selector: app=web", "http · 80 · TCP · http · 30080"}},
		{"networking.k8s.io", "ingresses", "Ingress", `{"spec":{"ingressClassName":"nginx","rules":[{"host":"shop.test","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"web","port":{"number":80}}}}]}}],"tls":[{"hosts":["shop.test"],"secretName":"tls-shop"}]}}`, []string{"Ingress class: nginx", "shop.test · / · Prefix · web:80", "shop.test · tls-shop"}},
		{"discovery.k8s.io", "endpointslices", "EndpointSlice", `{"addressType":"IPv4","endpoints":[{"addresses":["10.1.1.1"],"conditions":{"ready":false,"serving":true,"terminating":true},"nodeName":"worker-a","targetRef":{"kind":"Pod","name":"web","namespace":"shop"}}],"ports":[{"port":80,"protocol":"TCP"}]}`, []string{"Address type: IPv4", "10.1.1.1 · false · true · true · worker-a · Pod/shop/web", "80 · TCP"}},
		{"", "endpoints", "Endpoints", `{"subsets":[{"addresses":[{"ip":"10.1.1.1"}],"notReadyAddresses":[{"ip":"10.1.1.2"}],"ports":[{"port":80,"protocol":"TCP"}]}]}`, []string{"10.1.1.1 · true", "10.1.1.2 · false", "80 · TCP"}},
		{"networking.k8s.io", "networkpolicies", "NetworkPolicy", `{"spec":{"podSelector":{"matchLabels":{"app":"db"}},"policyTypes":["Ingress"],"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"team":"shop"}},"podSelector":{"matchLabels":{"app":"api"}}}],"ports":[{"protocol":"TCP","port":5432}]}]}}`, []string{"Pod selector: app=db", "Policy types: Ingress", "podSelector app=api AND namespaceSelector team=shop", "5432/TCP"}},
		{"", "nodes", "Node", `{"spec":{"unschedulable":true,"taints":[{"key":"dedicated","value":"db","effect":"NoSchedule"}]},"status":{"capacity":{"cpu":"8","memory":"16Gi"},"allocatable":{"cpu":"7500m","memory":"15Gi"},"nodeInfo":{"kubeletVersion":"v1.37.0"}}}`, []string{"Kubelet: v1.37.0", "Scheduling disabled: true", "cpu · 8 · 7500m", "dedicated · db · NoSchedule"}},
		{"", "namespaces", "Namespace", `{"status":{"phase":"Terminating"}}`, []string{"Phase: Terminating"}},
		{"", "persistentvolumeclaims", "PersistentVolumeClaim", `{"spec":{"storageClassName":"fast","volumeName":"pv-1","resources":{"requests":{"storage":"10Gi"}},"accessModes":["ReadWriteOnce"]},"status":{"phase":"Bound","capacity":{"storage":"10Gi"}}}`, []string{"Phase: Bound", "Volume: pv-1", "Requested storage: 10Gi", "Capacity: 10Gi", "Access modes: ReadWriteOnce"}},
		{"", "persistentvolumes", "PersistentVolume", `{"spec":{"capacity":{"storage":"20Gi"},"claimRef":{"name":"data","namespace":"shop"},"persistentVolumeReclaimPolicy":"Retain","csi":{"driver":"disk.example.io","volumeHandle":"disk-123"}}}`, []string{"Capacity: 20Gi", "Claim: shop/data", "Reclaim policy: Retain", "CSI driver: disk.example.io"}},
		{"storage.k8s.io", "storageclasses", "StorageClass", `{"provisioner":"disk.example.io","reclaimPolicy":"Delete","volumeBindingMode":"WaitForFirstConsumer","allowVolumeExpansion":true,"parameters":{"type":"ssd"}}`, []string{"Provisioner: disk.example.io", "Binding mode: WaitForFirstConsumer", "Allow expansion: true", "type · ssd"}},
		{"autoscaling", "horizontalpodautoscalers", "HorizontalPodAutoscaler", `{"spec":{"scaleTargetRef":{"kind":"Deployment","name":"web","apiVersion":"apps/v1"},"minReplicas":2,"maxReplicas":10,"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":70}}}]},"status":{"currentReplicas":3,"desiredReplicas":4}}`, []string{"Target kind: Deployment", "Target name: web", "Minimum replicas: 2", "Maximum replicas: 10", "Current replicas: 3", "Desired replicas: 4", "Resource · cpu ·  · 70%"}},
		{"", "resourcequotas", "ResourceQuota", `{"spec":{"hard":{"requests.cpu":"4","pods":"10"}},"status":{"used":{"requests.cpu":"1500m","pods":"3"}}}`, []string{"pods · 3 · not reported · 10", "requests.cpu · 1500m · not reported · 4"}},
		{"", "limitranges", "LimitRange", `{"spec":{"limits":[{"type":"Container","min":{"cpu":"50m"},"max":{"cpu":"2"},"defaultRequest":{"cpu":"100m"},"default":{"cpu":"1"}}]}}`, []string{"Container · cpu=50m · cpu=2 · cpu=100m · cpu=1"}},
		{"rbac.authorization.k8s.io", "roles", "Role", `{"rules":[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list"],"resourceNames":["web"]}]}`, []string{"core · pods · get, list · web"}},
		{"rbac.authorization.k8s.io", "rolebindings", "RoleBinding", `{"roleRef":{"kind":"Role","name":"reader","apiGroup":"rbac.authorization.k8s.io"},"subjects":[{"kind":"ServiceAccount","name":"viewer","namespace":"shop"}]}`, []string{"Role kind: Role", "Role name: reader", "ServiceAccount · viewer · shop"}},
		{"", "serviceaccounts", "ServiceAccount", `{"automountServiceAccountToken":false,"imagePullSecrets":[{"name":"registry"}]}`, []string{"Automount token: false", "Image pull secrets", "registry"}},
		{"admissionregistration.k8s.io", "validatingadmissionpolicies", "ValidatingAdmissionPolicy", `{"spec":{"failurePolicy":"Fail","matchConstraints":{"resourceRules":[{"operations":["CREATE"],"apiGroups":["apps"],"apiVersions":["v1"],"resources":["deployments"]}]},"validations":[{"expression":"object.spec.replicas <= 5","message":"Too many replicas"}]}}`, []string{"Failure policy: Fail", "CREATE · apps · v1 · deployments", "object.spec.replicas <= 5 · Too many replicas"}},
		{"admissionregistration.k8s.io", "mutatingadmissionpolicies", "MutatingAdmissionPolicy", `{"spec":{"reinvocationPolicy":"IfNeeded","mutations":[{"patchType":"ApplyConfiguration","applyConfiguration":{"expression":"Object{spec: Object.spec{replicas: 1}}"}}]}}`, []string{"Reinvocation policy: IfNeeded", "ApplyConfiguration · Object{spec: Object.spec{replicas: 1}}"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			object := summaryFixture(t, tc.object)
			text := summaryText(kubecommon.Resource{Group: tc.group, Name: tc.name, Kind: tc.kind}, object)
			for _, expected := range tc.want {
				if !strings.Contains(text, expected) {
					t.Errorf("missing %q in summary:\n%s", expected, text)
				}
			}
		})
	}
}

func TestPodSummaryKeepsLongEvictionMessageAfterContainerEvidence(t *testing.T) {
	pod := summaryFixture(t, `{"kind":"Pod","metadata":{"name":"checkout","namespace":"commerce"},"spec":{"containers":[{"name":"app","image":"checkout:v2"}]},"status":{"phase":"Failed","reason":"Evicted","containerStatuses":[{"name":"app","ready":false,"restartCount":12,"state":{"terminated":{"exitCode":137,"reason":"Error"}},"lastState":{"terminated":{"exitCode":137,"reason":"OOMKilled"}}}]}}`)
	message := "The node was low on resource: ephemeral-storage. Threshold quantity: 1073741824, available: 512Mi. " +
		"Container app was using 2048Mi, request is 0, has larger consumption of ephemeral-storage. " +
		"The Pod was evicted because its containers exceeded the local storage available on worker-a."
	if err := unstructured.SetNestedField(pod.Object, message, "status", "message"); err != nil {
		t.Fatal(err)
	}
	resource := kubecommon.Resource{Name: "pods", Kind: "Pod", Namespaced: true}
	sections := Summary(resource, pod)
	overview := findSummarySection(t, sections, "Pod overview")
	if got := findSummaryField(t, overview, "Reason"); got != "Evicted" {
		t.Fatalf("compact reason=%q", got)
	}
	for _, field := range overview.Fields {
		if field.Label == "Message" || strings.Contains(field.Value, message) {
			t.Fatal("long Pod message displaces first-screen container evidence")
		}
	}
	if got := findSummaryField(t, findSummarySection(t, sections, "Pod message"), "Message"); got != message {
		t.Fatal("full eviction message was lost")
	}
	text := summaryText(resource, pod)
	if strings.Index(text, message) < strings.Index(text, "Container · app") || strings.Index(text, message) < strings.Index(text, "OOMKilled (exit 137)") {
		t.Fatal("Pod message appears before container evidence")
	}
}

func summaryText(r kubecommon.Resource, object unstructured.Unstructured) string {
	lines := []string{r.Kind + " / " + object.GetName()}
	for _, section := range Summary(r, object) {
		lines = append(lines, "", section.Title)
		for _, field := range section.Fields {
			lines = append(lines, field.Label+": "+field.Value)
		}
		if len(section.Rows) > 0 {
			lines = append(lines, strings.Join(section.Columns, " · "))
			for _, row := range section.Rows {
				lines = append(lines, strings.Join(row, " · "))
			}
		}
	}
	return strings.Join(lines, "\n")
}
