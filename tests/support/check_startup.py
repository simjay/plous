"""Complete the OS launch check before applying interactive UI deadlines."""

from pathlib import Path
import signal
import subprocess
import sys
import time


def main():
    started = time.monotonic()
    try:
        result = subprocess.run([sys.argv[1], "--version"], capture_output=True, text=True, timeout=30)
    except subprocess.TimeoutExpired:
        print("Executable startup exceeded 30 seconds before UI tests. Check OS launch/security diagnostics.", file=sys.stderr)
        return 1
    except OSError as error:
        print(f"Cannot launch test executable: {error}", file=sys.stderr)
        return 1
    if result.returncode:
        status = f"exit code {result.returncode}"
        if result.returncode < 0:
            status = signal.Signals(-result.returncode).name
        print(f"Executable startup failed with {status}: {result.stderr.strip()}", file=sys.stderr)
        return 1
    expected = (Path(__file__).resolve().parents[2] / "cmd/plous/VERSION").read_text().strip()
    if result.stdout.strip() != expected:
        print("Test executable does not match cmd/plous/VERSION.", file=sys.stderr)
        return 1
    print(f"Executable startup verified in {time.monotonic() - started:.2f}s; UI deadlines remain unchanged.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
