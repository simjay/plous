package monitor

import (
	docedit "github.com/simjay/plous/internal/ui/editor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type YamlDocument struct {
	Document       *docedit.Model
	Identity, Text string
	Revealed       bool
	Source         *unstructured.Unstructured
	SourceVersion  string
}
