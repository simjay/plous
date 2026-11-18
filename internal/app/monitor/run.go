package monitor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	checkTimeout = 30 * time.Second
	stopTimeout  = 5 * time.Second
)

type config struct {
	Name, Kubeconfig, Context, Namespace string
	Targets                              []kubernetes.CheckTarget
	Interval                             time.Duration
	resume, intervalSet                  bool
}

type update struct {
	State   State
	Changed []Alert
	Started bool
}

func (c config) validate() error {
	if err := validateName(c.Name); err != nil {
		return err
	}
	if c.Context == "" || len(validation.IsDNS1123Label(c.Namespace)) != 0 {
		return errors.New("a context and valid namespace are required")
	}
	if c.Interval < time.Minute || c.Interval > time.Hour {
		return errors.New("check interval must be between 1m and 60m")
	}
	if len(c.Targets) > 32 {
		return errors.New("select at most 32 named targets")
	}
	for _, t := range c.Targets {
		if !slices.Contains(kubernetes.CheckTargetKinds(), t.Kind) {
			return errors.New("unsupported monitor target kind")
		}
		if len(validation.IsDNS1123Subdomain(t.Name)) != 0 {
			return fmt.Errorf("invalid %s name", t.Kind)
		}
	}
	return nil
}

func stop(ctx context.Context, root, name string) error {
	dir, err := directory(root, name)
	if err != nil {
		return err
	}
	state, err := readStored(dir)
	if errors.Is(err, os.ErrNotExist) || err == nil && (!state.Running || state.Control == "") {
		return errors.New("no monitor is running")
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()
	dial := net.Dialer{}
	conn, err := dial.DialContext(ctx, "unix", state.Control)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("the monitor process is not available")
	}
	if err := conn.Close(); err != nil {
		return err
	}

	// The worker saves its final state before releasing the lock that start and remove use.
	lock, err := os.Open(filepath.Join(dir, lockFile))
	if err != nil {
		return err
	}
	defer lock.Close()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		available, err := trySharedLock(lock)
		if err != nil || available {
			return err
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return errors.New("stop requested. Timed out waiting for the monitor to release its lock")
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func run(ctx context.Context, root string, cfg config, approveReplacement func(saved, requested State) error, report func(update)) (err error) {
	if !cfg.resume {
		if err := cfg.validate(); err != nil {
			return err
		}
	}
	dir, err := directory(root, cfg.Name)
	if err != nil {
		return err
	}
	if !cfg.resume {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	lock, err := lockMonitor(dir, cfg.Name)
	if cfg.resume && errors.Is(err, os.ErrNotExist) {
		return missingConfig(cfg.Name)
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	prior, loadErr := readStored(dir)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return loadErr
	}
	if cfg.resume {
		if loadErr != nil {
			return missingConfig(cfg.Name)
		}
		cfg.Kubeconfig, cfg.Context, cfg.Namespace = prior.Kubeconfig, prior.Context, prior.Namespace
		cfg.Targets = prior.Targets
		if !cfg.intervalSet {
			cfg.Interval = prior.Interval
		}
		if err := cfg.validate(); err != nil {
			return err
		}
	}
	target, err := kubecommon.ResolveTarget(cfg.Kubeconfig, cfg.Context, cfg.Namespace)
	if err != nil {
		return err
	}
	if cfg.resume && prior.Fingerprint != "" && prior.Fingerprint != target.Fingerprint {
		return errors.New("the saved Kubernetes connection changed. To replace it, start with --context and --namespace")
	}
	state := storedState{State: State{
		Name: cfg.Name, Context: cfg.Context, Namespace: cfg.Namespace, Fingerprint: target.Fingerprint,
		Targets: normalizeTargets(cfg.Targets), Interval: cfg.Interval,
	}, Kubeconfig: target.Paths}
	if loadErr == nil {
		if prior.Fingerprint == state.Fingerprint && prior.Namespace == state.Namespace && slices.Equal(normalizeTargets(prior.Targets), state.Targets) {
			state.Alerts, state.LastOK = prior.Alerts, prior.LastOK
		} else {
			if err := approveReplacement(prior.State, state.State); err != nil {
				return err
			}
			if err := target.CheckIdentity(); err != nil {
				return errors.New("the requested Kubernetes connection changed. Run start again to review it")
			}
		}
	}
	client, err := kubernetes.NewTarget(target, cfg.Namespace)
	if err != nil {
		return err
	}
	controlDir, err := os.MkdirTemp("/tmp", "plous-monitor-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(controlDir)
	address := filepath.Join(controlDir, "stop")
	listener, err := net.Listen("unix", address)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	controlDone := make(chan struct{})
	go func() {
		defer close(controlDone)
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
			cancel()
		}
	}()
	defer func() {
		listener.Close()
		<-controlDone
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	state.Control, state.Running, state.StartedAt = address, true, time.Now().UTC()
	state.NextCheck = state.StartedAt
	if err := save(dir, state); err != nil {
		return err
	}
	defer func() {
		state.Running, state.Control = false, ""
		state.NextCheck = time.Time{}
		err = errors.Join(err, save(dir, state))
	}()
	report(update{State: state.State, Started: true})
	rules := alertRules{}
	failures := 0
	for {
		started := time.Now()
		checkCtx, finish := context.WithTimeout(ctx, checkTimeout)
		check, checkErr := client.Check(checkCtx, state.Targets)
		finish()
		if ctx.Err() != nil {
			return nil
		}
		if checkErr != nil {
			check.Coverage = map[string]string{"target": "The pinned Kubernetes target could not be verified. Checks are suspended."}
		}
		changed := rules.apply(&state.State, check, time.Now().UTC())
		if len(state.Coverage) > 0 && len(check.Objects) == 0 {
			failures++
		} else {
			failures = 0
		}
		delay := min(cfg.Interval*time.Duration(1<<min(failures, 3)), time.Hour)
		nextCheck := started.Add(delay)
		state.NextCheck = nextCheck.UTC()
		if err := save(dir, state); err != nil {
			return err
		}
		report(update{State: state.State, Changed: changed})

		timer := time.NewTimer(time.Until(nextCheck))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func missingConfig(name string) error {
	return fmt.Errorf("no saved monitor named %q. Start it with --context and --namespace", name)
}

func normalizeTargets(targets []kubernetes.CheckTarget) []kubernetes.CheckTarget {
	t := slices.Clone(targets)
	slices.SortFunc(t, func(a, b kubernetes.CheckTarget) int {
		return strings.Compare(a.Kind+"/"+a.Name, b.Kind+"/"+b.Name)
	})
	return slices.Compact(t)
}
