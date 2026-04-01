---
name: plan-export
description: Generate polished planning documents with Mermaid diagrams, export as markdown and PDF.
---

# /plan-export — Planning Document Export

Generate a polished planning document with diagrams. Export as `.md` and optionally `.pdf`.

For the full document generation workflow, call `kb_get(name="plan/export")` if the detritus MCP server is available.

For Mermaid diagram syntax reference, call `kb_get(name="plan/diagrams")`.

## Quick Reference

1. Start with a `/plan` session to gather requirements and create the plan
2. Use `/plan-export` to format it as a polished document
3. Include Mermaid diagrams for architecture, data flow, state machines
4. Export to markdown file in the project
