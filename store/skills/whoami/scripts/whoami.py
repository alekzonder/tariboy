#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import main


if __name__ == "__main__":
    raise SystemExit(main("GET", "/tools/whoami", cli_args=sys.argv[1:]))
