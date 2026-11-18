package monitor

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/term"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

const Usage = "plous monitor start|status|stop|remove NAME [options]\n       plous monitor list [options]"

var errDeclined = errors.New("confirmation declined")

func Command(ctx context.Context, args []string, dataDir string, input *os.File, output, diagnostics io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(output, "Usage:", Usage)
		return 0
	}
	action := args[0]
	if action != "start" && action != "status" && action != "stop" && action != "remove" && action != "list" {
		fmt.Fprintln(diagnostics, "Usage:", Usage)
		return 2
	}
	flags := flag.NewFlagSet("plous monitor "+action, flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.Usage = func() {
		name := " NAME"
		if action == "list" {
			name = ""
		}
		fmt.Fprintf(diagnostics, "Usage: plous monitor %s%s [options]\n", action, name)
		flags.PrintDefaults()
	}
	flags.StringVar(&dataDir, "data-dir", dataDir, "Local Plous data directory (default ~/.plous)")
	cfg := config{}
	replace := false
	args = args[1:]
	if action != "list" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cfg.Name, args = args[0], args[1:]
	}
	if action == "start" {
		flags.StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Kubeconfig file")
		flags.StringVar(&cfg.Context, "context", "", "Required Kubernetes context")
		flags.StringVar(&cfg.Namespace, "namespace", "", "Required Kubernetes namespace")
		flags.DurationVar(&cfg.Interval, "every", 5*time.Minute, "Time between checks (1m to 60m)")
		flags.BoolVar(&replace, "replace", false, "Approve replacing a stopped monitor's scope and clearing its saved alerts")
		for _, kind := range kubernetes.CheckTargetKinds() {
			flags.Func(strings.ToLower(kind), "Check a specific "+kind+" (repeatable)", func(name string) error {
				cfg.Targets = append(cfg.Targets, kubernetes.CheckTarget{Kind: kind, Name: name})
				return nil
			})
		}
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "Unexpected positional arguments")
		return 2
	}
	if action != "list" {
		if err := validateName(cfg.Name); err != nil {
			fmt.Fprintln(diagnostics, "Plous:", err)
			fmt.Fprintln(diagnostics, "Usage:", Usage)
			return 2
		}
	}
	root, err := Root(dataDir)
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	switch action {
	case "list":
		states, listErr := List(root)
		err = listErr
		if len(states) == 0 && err == nil {
			fmt.Fprintln(output, "No saved monitors.")
		}
		for _, state := range states {
			lastOK := "no complete check"
			if !state.LastOK.IsZero() {
				lastOK = "last complete check " + state.LastOK.Format(time.RFC3339)
			}
			fmt.Fprintf(output, "%s · %d open · %s\n", StatusLabel(state), state.OpenAlerts(), lastOK)
		}
	case "start":
		cfg.resume = true
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "data-dir", "replace":
			case "every":
				cfg.intervalSet = true
			default:
				cfg.resume = false
			}
		})
		approveReplacement := func(saved, requested State) error {
			if replace {
				return nil
			}
			return confirm(ctx, input, diagnostics, replacementPrompt(saved, requested), "confirm the replacement in a terminal or use --replace")
		}
		stopHelpers := kubernetes.StartCredentialHelpers()
		defer stopHelpers()
		previous := map[string]string{}
		err = run(ctx, root, cfg, approveReplacement, func(update update) {
			if update.Started {
				fmt.Fprintln(output, StatusLabel(update.State))
				return
			}
			if !maps.Equal(previous, update.State.Coverage) {
				if len(update.State.Coverage) == 0 {
					fmt.Fprintln(output, "Coverage restored.")
				}
				writeCoverage(output, update.State)
				previous = update.State.Coverage
			}
			for _, item := range update.Changed {
				transition := item.State
				if item.WorsenedAt.Equal(update.State.LastCheck) {
					transition = "worsened"
				}
				fmt.Fprintf(output, "%s  %s  %s  %s\n", update.State.LastCheck.Format(time.RFC3339), update.State.Context+"/"+update.State.Namespace, strings.ToUpper(transition), AlertLabel(item))
			}
		})
		if err == nil {
			fmt.Fprintf(output, "Monitor %s stopped.\n", cfg.Name)
		}
	case "stop":
		err = stop(ctx, root, cfg.Name)
		if err == nil {
			fmt.Fprintf(output, "Monitor %s stopped.\n", cfg.Name)
		}
	case "remove":
		err = remove(root, cfg.Name, func() error {
			prompt := fmt.Sprintf("Delete %s and its saved alerts?", cfg.Name)
			return confirm(ctx, input, diagnostics, prompt, "confirm monitor removal in a terminal")
		})
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(output, "No saved monitor named %s.\n", cfg.Name)
			return 0
		}
		if err == nil {
			fmt.Fprintf(output, "Monitor %s removed.\n", cfg.Name)
		}
	case "status":
		state, statusErr := status(root, cfg.Name)
		err = statusErr
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(output, "No saved monitor named %s.\n", cfg.Name)
			return 0
		}
		if err != nil {
			break
		}
		fmt.Fprintln(output, StatusLabel(state))
		if !state.LastCheck.IsZero() {
			fmt.Fprintln(output, "Last check:", state.LastCheck.Format(time.RFC3339))
		}
		if !state.LastOK.IsZero() {
			fmt.Fprintln(output, "Last complete check:", state.LastOK.Format(time.RFC3339))
		}
		if state.Running && !state.NextCheck.IsZero() {
			label := "Next check:"
			if state.Overdue(time.Now()) {
				label = "Overdue check:"
			}
			fmt.Fprintln(output, label, state.NextCheck.Format(time.RFC3339))
		}
		writeCoverage(output, state)
		for _, item := range state.Alerts {
			if item.State == "open" {
				fmt.Fprintln(output, AlertLabel(item))
			}
		}
	}
	switch {
	case errors.Is(err, errDeclined):
		fmt.Fprintln(output, "Canceled.")
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(diagnostics, "\nCanceled.")
		return 130
	case err != nil:
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	return 0
}

func replacementPrompt(saved, requested State) string {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Saved scope      %s / every %s\nRequested scope  %s / every %s\n",
		ui.OneLine(scopeLabel(saved)), saved.Interval, ui.OneLine(scopeLabel(requested)), requested.Interval)
	if saved.Context == requested.Context && saved.Fingerprint != requested.Fingerprint {
		fmt.Fprintln(&prompt, "The Kubernetes connection for this context has changed.")
	}
	fmt.Fprintf(&prompt, "Saved alerts     %d\nReplace %s and clear its saved alerts?", len(saved.Alerts), requested.Name)
	return prompt.String()
}

func confirm(ctx context.Context, input *os.File, output io.Writer, prompt, terminalRequired string) error {
	if input == nil || !term.IsTerminal(input.Fd()) {
		return errors.New(terminalRequired)
	}
	reader, err := uv.NewCancelReader(input)
	if err != nil {
		return err
	}
	defer reader.Close()
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		reader.Cancel()
		close(canceled)
	})
	defer func() {
		if !stop() {
			<-canceled
		}
	}()
	if _, err := fmt.Fprint(output, prompt, " [y/N] "); err != nil {
		return err
	}
	line, err := bufio.NewReaderSize(reader, 64).ReadSlice('\n')
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, io.EOF) || errors.Is(err, bufio.ErrBufferFull) {
		return errDeclined
	}
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(string(line))) {
	case "y", "yes":
		return nil
	default:
		return errDeclined
	}
}

func StatusLabel(state State) string {
	status := "stopped"
	if state.Running {
		status = "running"
	}
	return fmt.Sprintf("Monitor %s (%s): %s / every %s", state.Name, status, scopeLabel(state), state.Interval)
}

func scopeLabel(state State) string {
	targets := make([]string, 0, len(state.Targets))
	for _, target := range state.Targets {
		targets = append(targets, target.Kind+"/"+target.Name)
	}
	if len(targets) == 0 {
		targets = append(targets, "all Deployments")
	}
	return fmt.Sprintf("%s / %s / %s", state.Context, state.Namespace, strings.Join(targets, ", "))
}

func AlertLabel(item Alert) string {
	container := ""
	if item.Container != "" {
		container = " / " + item.Container
	}
	baseline := ""
	if item.PresentAtStart {
		baseline = " (present at start)"
	}
	return fmt.Sprintf("%s/%s%s: %s. %s%s", item.Kind, item.Name, container, item.Signal, item.Detail, baseline)
}

func writeCoverage(output io.Writer, state State) {
	for _, k := range slices.Sorted(maps.Keys(state.Coverage)) {
		fmt.Fprintf(output, "Coverage / %s: %s\n", k, state.Coverage[k])
	}
}
