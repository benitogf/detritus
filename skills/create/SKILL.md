---
name: create
description: Scaffold a new project — backend service, full-stack web app, or desktop app. Interactive Q&A to determine project structure.
---

# /create — Project Scaffolding

Interactive workflow to scaffold a new project using the ooo ecosystem.

For the full scaffolding workflow with code templates, call `kb_get(name="scaffold/create")` if the detritus MCP server is available.

## Questions to Ask

1. **Project name?**
2. **Backend only or full-stack (with React UI)?**
3. **Authentication needed?** (adds github.com/benitogf/auth)
4. **Desktop app?** (adds webview wrapper)

## Default Stack

- **Backend**: ooo + ko (LevelDB storage) + memory fallback
- **Frontend** (if full-stack): React + ooo-client
- **Auth** (if needed): JWT via benitogf/auth
- **Desktop** (if needed): webview wrapper
