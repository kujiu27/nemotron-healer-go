#!/usr/bin/env python3
"""Check that every relative link in the top-level docs resolves to a file.

Round-12 lesson: a worktree 'git add -A' silently deleted docs/PERF.md and
README kept 4 dead receipt links for three rounds. This guard makes any
dead relative link a CI failure.
"""
import os
import re
import sys

DOCS = ["README.md", "DEVPOST_SUBMISSION.md", "GRAND_PRIZE_GAP_AUDIT.md"]
broken = []
for doc in DOCS:
    if not os.path.exists(doc):
        broken.append((doc, "FILE ITSELF MISSING"))
        continue
    text = open(doc, encoding="utf-8").read()
    for m in re.finditer(r"\]\(([^)#?]+)(?:#[^)]*)?\)", text):
        link = m.group(1).strip()
        if link.startswith(("http://", "https://", "mailto:")):
            continue
        if not os.path.exists(link):
            broken.append((doc, link))

if broken:
    for doc, link in broken:
        print(f"BROKEN LINK: {doc} -> {link}")
    sys.exit(1)
print("All relative doc links resolve.")
