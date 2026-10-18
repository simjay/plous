package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentTargetMergeAndImmutableConfiguration(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "contexts"), filepath.Join(dir, "access")
	first := "apiVersion: v1\nkind: Config\ncurrent-context: selected\ncontexts:\n- name: selected\n  context: {cluster: cluster, user: user, namespace: apps}\n"
	second := "apiVersion: v1\nkind: Config\ncurrent-context: ignored\nclusters:\n- name: cluster\n  cluster: {server: 'https://first.invalid'}\nusers:\n- name: user\n  user: {token: secret-one}\n"
	if err := os.WriteFile(a, []byte(first), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(second), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", strings.Join([]string{a, b}, string(os.PathListSeparator)))
	inventory, err := DiscoverCurrent("")
	if err != nil || inventory.Current != "selected" || len(inventory.Contexts) != 1 {
		t.Fatalf("inventory=%+v err=%v", inventory, err)
	}
	target, err := ResolveTarget("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Context != "selected" || target.Namespace != "apps" {
		t.Fatal(target.Context, target.Namespace)
	}
	if err = os.WriteFile(b, []byte(strings.ReplaceAll(second, "secret-one", "secret-two")), 0600); err != nil {
		t.Fatal(err)
	}
	renewed, err := ResolveTarget("", "", "")
	if err != nil || target.Fingerprint != renewed.Fingerprint {
		t.Fatal("credential rotation changed identity", err)
	}
	if err = os.WriteFile(b, []byte(strings.ReplaceAll(second, "first.invalid", "second.invalid")), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := ResolveTarget("", "", "")
	if err != nil || target.Fingerprint == changed.Fingerprint {
		t.Fatal("replaced target retained identity", err)
	}
	config, err := target.Loader("").ClientConfig()
	if err != nil || config.Host != "https://first.invalid" || config.BearerToken != "secret-one" {
		t.Fatal("captured configuration changed", err)
	}
	if target.CheckIdentity() == nil {
		t.Fatal("write did not detect changed cluster")
	}
	config.Host = "https://mutated.invalid"
	again, _ := target.Loader("").ClientConfig()
	if again.Host != "https://first.invalid" {
		t.Fatal("loader exposed mutable captured config")
	}
}

func TestCurrentDiscoveryDoesNotExecuteCredentialsOrChooseFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	data := "apiVersion: v1\nkind: Config\ncontexts:\n- name: other\n  context: {cluster: c, user: u}\nclusters:\n- name: c\n  cluster: {server: 'https://cluster.invalid'}\nusers:\n- name: u\n  user:\n    exec: {apiVersion: client.authentication.k8s.io/v1, command: '/does-not-exist', interactiveMode: Never}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if inventory, err := DiscoverCurrent(path); err != nil || len(inventory.Contexts) != 1 {
		t.Fatal(inventory, err)
	}
	if _, err := ResolveTarget(path, "", ""); err == nil {
		t.Fatal("missing default selected arbitrary context")
	}
	if target, err := ResolveTarget(path, "other", ""); err != nil || target.Namespace != "default" {
		t.Fatal("local resolution executed plugin", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != data {
		t.Fatal("discovery changed kubeconfig")
	}
}

func TestTargetPreservesRelativeTrustAndSeparatesTransportChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), []byte("captured certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	data := "apiVersion: v1\nkind: Config\ncurrent-context: one\ncontexts:\n- name: one\n  context: {cluster: c}\nclusters:\n- name: c\n  cluster: {server: 'https://example.invalid', certificate-authority: ca.pem, proxy-url: 'http://user:secret-one@proxy.invalid:8080'}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := ResolveTarget(path, "", "apps")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(data, "secret-one", "secret-two")), 0600); err != nil {
		t.Fatal(err)
	}
	renewed, err := ResolveTarget(path, "", "another")
	if err != nil || first.Fingerprint != renewed.Fingerprint {
		t.Fatal("namespace or proxy credential changed scope", err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(data, "proxy.invalid", "other.invalid")), 0600); err != nil {
		t.Fatal(err)
	}
	replaced, err := ResolveTarget(path, "", "")
	if err != nil || first.Fingerprint == replaced.Fingerprint {
		t.Fatal("transport change reused scope", err)
	}
	if err := os.Remove(filepath.Join(dir, "ca.pem")); err != nil {
		t.Fatal(err)
	}
	config, err := first.Loader("").ClientConfig()
	if err != nil || string(config.CAData) != "captured certificate" {
		t.Fatal("relative certificate was not captured", err)
	}
}
