package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const MaxWorkloadLogSources = 32

type WorkloadLogStatus struct {
	Pods, Sources, Active, Finished, Failed, Waiting, Skipped int
	Containers                                                []string
	MembershipError                                           string
}

type WorkloadLogEvent struct {
	Text   string
	Status *WorkloadLogStatus
}

type workloadLogSource struct {
	options LogOptions
	key     string
	ready   bool
}

type workloadLogStream struct {
	cancel                  context.CancelFunc
	connected, done, failed bool
}

type workloadStreamEvent struct {
	stream          *workloadLogStream
	text            string
	connected, done bool
	err             error
}

// WorkloadLogs follows controller ownership and retains no output itself.
// Empty Container selects application containers; a name also permits init and
// ephemeral containers. Previous logs use a single membership snapshot.
func (c *Client) WorkloadLogs(ctx context.Context, r Resource, object *unstructured.Unstructured, resources []Resource, options LogOptions, emit func(WorkloadLogEvent)) error {
	return workloadLogs(ctx, options, 3*time.Second, func(ctx context.Context) ([]unstructured.Unstructured, error) {
		return c.PodTargets(ctx, r, object, resources)
	}, c.Logs, emit)
}

func workloadSources(pods []unstructured.Unstructured, options LogOptions) ([]workloadLogSource, []string, int) {
	var sources []workloadLogSource
	names := map[string]bool{}
	total := 0
	for _, pod := range pods {
		for _, name := range Containers(pod) {
			names[name] = true
		}
		containers := Maps(pod.Object, "spec", "containers")
		if options.Container != "" {
			containers = nil
			if slices.Contains(Containers(pod), options.Container) {
				containers = []map[string]any{{"name": options.Container}}
			}
		}
		for _, container := range containers {
			name := Value(container, "name")
			if name == "" || pod.GetUID() == "" {
				continue
			}
			total++
			if len(sources) >= MaxWorkloadLogSources {
				continue
			}
			ready, instance := false, ""
			for _, field := range []string{"containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"} {
				for _, status := range Maps(pod.Object, "status", field) {
					if Value(status, "name") != name {
						continue
					}
					instance = Value(status, "restartCount")
					state := nestedMap(status, "state")
					_, running := state["running"]
					_, terminated := state["terminated"]
					ready = running || terminated || instance != "" && instance != "0" || options.Previous
					if !options.Previous {
						instance += "/" + Value(status, "containerID")
					}
				}
			}
			source := options
			source.Namespace, source.Pod, source.UID, source.Container = pod.GetNamespace(), pod.GetName(), pod.GetUID(), name
			sources = append(sources, workloadLogSource{source, string(pod.GetUID()) + "/" + name + "/" + instance, ready})
		}
	}
	available := make([]string, 0, len(names))
	for name := range names {
		available = append(available, name)
	}
	slices.Sort(available)
	return sources, available, total
}

func workloadLogs(parent context.Context, options LogOptions, interval time.Duration, targets func(context.Context) ([]unstructured.Unstructured, error), logs func(context.Context, LogOptions, func(string)) error, emit func(WorkloadLogEvent)) error {
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	type membership struct {
		pods []unstructured.Unstructured
		err  error
	}
	memberships := make(chan membership, 1)
	events := make(chan workloadStreamEvent, 64)
	streams := map[string]*workloadLogStream{}
	send := func(event workloadStreamEvent) {
		select {
		case events <- event:
		case <-ctx.Done():
		}
	}
	fetch := func() {
		workers.Add(1)
		go func() {
			defer workers.Done()
			pods, err := targets(ctx)
			select {
			case memberships <- membership{pods, err}:
			case <-ctx.Done():
			}
		}()
	}
	fetch()
	timer := time.NewTimer(interval)
	defer timer.Stop()
	timer.Stop()
	var status WorkloadLogStatus
	loaded := false
	publish := func() {
		state := status
		state.Active, state.Finished, state.Failed = 0, 0, 0
		for _, stream := range streams {
			switch {
			case stream.failed:
				state.Failed++
			case stream.done:
				state.Finished++
			case stream.connected:
				state.Active++
			}
		}
		emit(WorkloadLogEvent{Status: &state})
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			fetch()
		case member := <-memberships:
			loaded = true
			if member.err != nil {
				if errors.Is(member.err, TargetChangedError()) || apierrors.IsNotFound(member.err) {
					return member.err
				}
				status.MembershipError = ErrorText(member.err)
				publish()
				timer.Reset(interval)
				continue
			}
			sources, names, total := workloadSources(member.pods, options)
			status = WorkloadLogStatus{Pods: len(member.pods), Sources: total, Skipped: max(0, total-len(sources)), Containers: names}
			wanted := map[string]bool{}
			for _, source := range sources {
				if source.ready {
					wanted[source.key] = true
				} else {
					status.Waiting++
				}
			}
			for key, stream := range streams {
				if !wanted[key] {
					stream.cancel()
					delete(streams, key)
				}
			}
			for _, source := range sources {
				if !source.ready || streams[source.key] != nil {
					continue
				}
				streamCtx, stop := context.WithCancel(ctx)
				stream := &workloadLogStream{cancel: stop}
				streams[source.key] = stream
				workers.Add(1)
				go func() {
					defer workers.Done()
					defer stop()
					source.options.OnConnect = func() { send(workloadStreamEvent{stream: stream, connected: true}) }
					err := logs(streamCtx, source.options, func(text string) {
						for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
							send(workloadStreamEvent{stream: stream, text: "[" + source.options.Pod + "/" + source.options.Container + "] " + line + "\n"})
						}
					})
					if err != nil && streamCtx.Err() == nil {
						send(workloadStreamEvent{stream: stream, text: fmt.Sprintf("[%s/%s] %s\n", source.options.Pod, source.options.Container, LogErrorText(err, options.Previous))})
					}
					send(workloadStreamEvent{stream: stream, done: true, err: err})
				}()
			}
			publish()
			if !options.Previous {
				timer.Reset(interval)
			}
		case event := <-events:
			// Removed or replaced Pod streams cannot append late output.
			current := false
			for _, stream := range streams {
				if stream == event.stream {
					current = true
					break
				}
			}
			if !current {
				continue
			}
			if event.connected {
				event.stream.connected = true
			}
			if event.done {
				event.stream.done = true
				event.stream.failed = event.err != nil
			}
			if event.text != "" {
				emit(WorkloadLogEvent{Text: event.text})
			}
			if event.connected || event.done {
				publish()
			}
		}
		if loaded && options.Previous && status.MembershipError == "" {
			done := true
			for _, stream := range streams {
				if !stream.done {
					done = false
					break
				}
			}
			if done {
				return nil
			}
		}
	}
}
