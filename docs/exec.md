# Container terminals

[Documentation](README.md) · [Kubernetes actions](kubernetes-actions.md)

Exec runs a command inside a container. Use it when inspecting the resource and its logs is not enough. Commands run with the container's permissions and can change its files or processes.

## Open the view

1. Open a Pod or supported workload's details and press **x**.
2. Choose a running Pod/container when prompted.
3. Select **sh**, **bash**, or **Custom command** with Up/Down and Enter. Presets show `/bin/sh` or `/bin/bash`; neither is guaranteed to exist in the image. Custom command opens the bordered field; Enter finishes editing it.
4. Select **Start** and press Enter. Only the displayed command runs. Plous does not try other shells automatically.

## Switch between terminal input and Plous controls

The session stays in the main panel. **Terminal input active** means ordinary keys, Esc, Tab, colons, and paste go to the remote process. **Ctrl+]** switches to **Plous controls active**. **Ctrl+C always saves and exits Plous**, including while terminal input is active.

## Read earlier output

Plous Log Viewer holds a snapshot for reading while the process continues. Arrows, **j/k**, Page Up/Down, and Home move through retained output. **End** refreshes the snapshot to the latest output. Use **/** to search, **n/N** for matches, **v** to select, and **y** to copy selected text or the current line. Clipboard support depends on your terminal. Esc clears selection or search before closing the session.

Press **r** to resume input, **t** to send an interrupt to the remote process, or **x** to close. Completed sessions keep output and the full sanitized error available to scroll and search; Resume and Interrupt are unavailable. After completion, **e** opens the command editor, which also provides shell choices, and uppercase **R** explicitly retries the same command. Canceling the launcher restores the completed output. Retry checks the original Pod identity again, so a replacement Pod is not silently substituted.

## Retention and session lifetime

Exec retains up to 5,000 history rows plus its current screen. Review snapshots also have a 2 MiB text-and-style budget, with an indication if earlier rows were removed. Alternate-screen programs expose their current screen separately from shell history. Review copies terminal screen rows without color codes; terminal screen wrapping remains part of those rows.

Resizing is forwarded to the remote terminal. Closing detaches its connection; a detached process may continue running in the container. Output and commands are not automatically saved as transcripts.

See [Keyboard shortcuts](keymap.md) for the complete key reference.
