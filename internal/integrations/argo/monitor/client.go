// Package monitor reads Argo controller observations through a captured Kubernetes reader.
package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/simjay/plous/internal/integrations/argo/common"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	transport "github.com/simjay/plous/internal/integrations/gitops/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func roots() []gitops.Reference {
	return []gitops.Reference{{Group: common.Group, Kind: "Application"}, {Group: common.Group, Kind: "ApplicationSet"}, {Group: common.Group, Kind: "AppProject"}}
}

type Client struct {
	*transport.Client
	reader transport.Reader
}

func New(reader transport.Reader, sections ...gitops.Section) *Client {
	section := gitops.Applications
	if len(sections) > 0 {
		section = gitops.NormalizeSection(gitops.ArgoCD, sections[0])
	}
	ref := roots()[0]
	for _, candidate := range roots() {
		if gitops.SectionFor(candidate) == section {
			ref = candidate
		}
	}
	return &Client{Client: transport.New(reader, transport.Definition{Roots: []gitops.Reference{ref}, Allows: allows, SupportsVersion: SupportsVersion, Parse: Parse}), reader: reader}
}

func allows(ref gitops.Reference) bool {
	for _, root := range roots() {
		if root.Group == ref.Group && root.Kind == ref.Kind {
			return true
		}
	}
	return false
}
func Supports(catalog []kubecommon.Resource) bool { return transport.Supports(catalog, roots()) }
func SupportsVersion(r kubecommon.Resource) bool {
	return r.Group == common.Group && r.Version == "v1alpha1"
}

func (c *Client) Load(ctx context.Context, namespace string) gitops.Snapshot {
	catalog, warning, err := c.reader.Discover(ctx)
	if err != nil {
		failure := transport.ClassifyError(err)
		return gitops.Snapshot{State: failure.State, Message: failure.Message, ReadAt: time.Now()}
	}
	return c.LoadWithCatalog(ctx, namespace, catalog, warning)
}

func (c *Client) LoadWithCatalog(ctx context.Context, namespace string, catalog []kubecommon.Resource, warning string) gitops.Snapshot {
	snapshot := c.Client.LoadWithCatalog(ctx, namespace, catalog, warning)
	c.relatedApplications(ctx, &snapshot, catalog)
	return snapshot
}

func (c *Client) Get(ctx context.Context, ref gitops.Reference) (gitops.Item, error) {
	item, err := c.Client.Get(ctx, ref)
	if err != nil || item.Ref.Kind == "Application" {
		return item, err
	}
	catalog, _, err := c.reader.Discover(ctx)
	if err != nil {
		item.RelatedMessage = transport.ClassifyError(err).Message
		return item, nil
	}
	snapshot := gitops.Snapshot{Items: []gitops.Item{item}, ReadAt: item.ReadAt}
	c.relatedApplications(ctx, &snapshot, catalog)
	return snapshot.Items[0], nil
}

func (c *Client) relatedApplications(ctx context.Context, snapshot *gitops.Snapshot, catalog []kubecommon.Resource) {
	// Namespace-scoped reads avoid mixing independent Argo installations. Projects
	// may also govern Applications in other namespaces; this is not a global count.
	namespaces := map[string][]int{}
	for i, item := range snapshot.Items {
		if item.Ref.Kind == "ApplicationSet" || item.Ref.Kind == "AppProject" {
			namespaces[item.Ref.Namespace] = append(namespaces[item.Ref.Namespace], i)
		}
	}
	resource, exists := gitops.FindResource(catalog, roots()[0])
	for namespace, indices := range namespaces {
		var apps []gitops.Item
		owners := map[string][]string{}
		message := "Applications in namespace " + namespace + ". Other namespaces are not included."
		complete := true
		if !exists || !SupportsVersion(resource) {
			message, complete = "Application API unavailable. Related applications could not be read.", false
		} else {
			collection, err := c.reader.List(ctx, resource, namespace, "")
			if err != nil {
				message, complete = transport.ClassifyError(err).Message, false
			} else {
				if collection.Truncated {
					message, complete = "Application list is incomplete in namespace "+namespace+".", false
				}
				for _, object := range collection.Objects {
					object.SetKind(resource.Kind)
					object.SetAPIVersion(resource.GVR().GroupVersion().String())
					app := ParseApplication(object, snapshot.ReadAt)
					apps = append(apps, app)
					for _, owner := range object.GetOwnerReferences() {
						if owner.APIVersion == common.Group+"/v1alpha1" && owner.Kind == "ApplicationSet" {
							owners[app.Ref.Key()] = append(owners[app.Ref.Key()], string(owner.UID))
						}
					}
				}
			}
		}
		gitops.SortItems(apps)
		for _, index := range indices {
			item := &snapshot.Items[index]
			item.RelatedMessage = message
			for _, app := range apps {
				matches := item.Ref.Kind == "AppProject" && gitops.FieldValue(app, "Project") == item.Ref.Name
				if item.Ref.Kind == "ApplicationSet" && item.UID != "" {
					for _, uid := range owners[app.Ref.Key()] {
						if uid == item.UID {
							matches = true
						}
					}
				}
				if matches {
					item.Related = append(item.Related, gitops.ManagedResource{Ref: app.Ref, Sync: app.Sync, Health: app.Health})
				}
			}
			if complete && (item.Ref.Kind != "ApplicationSet" || item.UID != "") {
				gitops.AddField(item, "Applications", fmt.Sprint(len(item.Related)))
			} else if item.UID == "" && item.Ref.Kind == "ApplicationSet" {
				item.RelatedMessage = "ApplicationSet identity is unavailable. Related applications could not be matched."
			}
		}
	}
}
