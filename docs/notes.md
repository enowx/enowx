# Notes

The rail's **Notes** page keeps pages of notes on the connected host: plans, checklists, what an agent found. Pages nest under one another, as a notebook's, and every app connected to the host shows the same ones as they change.

## Writing

A page has an icon, a title and a body, written a block at a time:

- Each line is a block, shown as it reads; a click opens it as text. Enter starts the next block, Backspace at a block's start joins it to the one before, and Up and Down move between them.
- Markdown typed at a block's start makes it another kind: `# `, `## ` and `### ` headings, `- ` a bulleted list, `1. ` a numbered one, `[] ` a to-do, `> ` a quote; `---` and Enter a divider, ` ``` ` and Enter a code block.
- `/` in an empty block lists the kinds of block, narrowed by what follows it (`/todo`); Enter or a click picks one, and a table among them.
- Tab and Shift+Tab indent a list item; Enter on an empty item ends the list. A to-do's box ticks it.
- Text keeps Markdown's **bold**, *italics*, `code`, ~~strikethrough~~ and links. Lines pasted make blocks.
- Dragging a block, or the handle beside the block the pointer is on, moves it before the one it is dropped on.
- `/icon` puts an icon in the text, picked as a page's is: `:icon[lucide:check]`, drawn in the line. `/drawing` makes a code block of language `svg`, shown as the drawing it is (cleaned as icons are).

The page saves itself a moment after a change. A page an agent or another app changes while it isn't being edited here is read again.

**Undo / redo:** Ctrl+Z undoes an edit on Windows and Linux; Ctrl+Y or Ctrl+Shift+Z redoes it. On macOS use ⌘Z and ⇧⌘Z. The matching buttons are in the page's header. Undo includes text, title, icon, block changes and image insertion, deletion and renaming, including text beside an image and Markdown mode. Consecutive typing is grouped. History is kept while the page remains open, up to 64 steps with a memory limit; changes read again from another app reset it. These shortcuts apply to the note's editing area: search fields, dialogs, the agent's chat and terminals keep their own shortcuts. Read mode does not undo or redo edits.

**Add image…** (the image button in the page's header) adds a picture from this device. You can also paste a copied image or screenshot with Ctrl+V / ⌘V while typing in the body or Markdown view. The picture appears at the end of the page; in Markdown it is an ordinary image link. PNG, JPEG, GIF, WebP and BMP are supported, up to 8 MB and 40 megapixels. The image is uploaded to the connected host and kept with the note's assets, so it still appears after reopening and is included in sync and exports. Read mode keeps image insertion disabled.

Text on the same line as an image stays visible beside it (below it on narrow pages). Clicking the image block edits that text while keeping the picture visible; Markdown mode still exposes the original image link. Type `@` in a text block or beside a picture to pick an image already on this page, filter by its name or file name, then press Enter or Tab. The link button under a picture also inserts its mention in a new block. Mentions are ordinary Markdown links, such as `[@Image 1](assets/<ID>/pasted-image.png)`; clicking one opens the picture inside Notes.

The area beside an image supports the same `/` block menu and Markdown shortcuts as the rest of the page, including headings, to-dos, lists and code. Enter continues with another block beside the picture. Structured content is saved using the existing `:::columns` layout. Right-click the picture and choose **Rename image…** to change its display name and mentions on this page; the asset reference stays the same.

The trash button under a picture, or **Delete image** in its right-click menu, removes that picture from the page. Backspace in its empty companion text also removes it, as do Backspace at the start of the block after it and Delete at the end of the block before it. Companion text and its block formatting remain in the note as ordinary blocks. The stored asset stays available to existing mentions and history.

Full paths to files in `notes/assets/<ID>/` also resolve through the note's host, including Windows paths and `file:///` links. In Markdown, wrap a path containing spaces in angle brackets: `![Image](<C:/Users/Your Name/.enowx-app/notes/assets/ID/image.png>)`.

The page's header switches between **Edit** (blocks open as text where they show), **Read** (clicks follow links, pick tabs and open toggles, and nothing opens for typing) and **Markdown** (the whole page as its Markdown, to write as text).

## Layout

A page needn't be one long text. Layout blocks are written in its Markdown as directives, which the page shows laid out and other editors show as plain lines (`core/noteblocks`):

```
:::tabs
::tab Summary
- [ ] sketch it
::tab Tests
- [ ] try it on a VPS
:::

:::toggle More about it
detail most readers skip
:::

:::callout warning Careful
Don't ship on a Friday {{High}}
:::

:::columns
::col
Left
::col
Right
:::

:::cards
::card Sync
One place for everything {{Done}}
::card Vault
Secrets {{WIP}}
:::

::toc
::progress
::pages
::spacer
::embed https://example.com/shot.png
```

- A block opens with `:::name` on a line of its own and closes with `:::`; blocks nest. In tabs, columns and cards, `::tab Title`, `::col` and `::card Title` start each part.
- Tabs show one part at a time; a toggle is folded until opened; columns sit side by side, one under another on a narrow page; callouts are `info`, `tip`, `success`, `warning` or `danger`; cards make a grid as wide as the page lets. Which tab is picked, which toggle is open and how the outline is folded are remembered on the device (the app's state), not written in the note.
- `::toc` lists the page's headings, `::progress` is a bar of the to-dos done in its section, `::pages` lists the pages inside, `::spacer` makes room, `::embed URL` shows an image, a link as a card with its page's title, or a canvas (`canvas:ID`).
- In a line, `{{High}}` is a badge (done green, high and blocked red, in progress amber), `[[Page]]` links to a page, `[[#Section]]` to a section of this one, `[[Page#Section]]` to one of another, `[[target|text]]` with other words.
- A layout block shows laid out; its to-dos tick where they show, and a click elsewhere on it opens it as Markdown. `/` lists the blocks too (`/tabs`, `/toggle`, `/columns`, `/callout`, `/cards`, `/toc`, `/progress`, `/pages`, `/embed`, `/spacer`).
- A table sorts by a click on a column's header (again: the other way, a third time: as written), and one of more than five rows gets a filter.

## Getting around a page

- **On this page**, beside a page wide enough, is its outline: the headings and the titled parts of layout blocks, each with its to-dos done, the section read now lit. A click scrolls there (opening the tab or toggle it is in); a heading's chevron folds the sections under it, and 1, 2, 3 show the headings down to that level. The page's to-dos show as a bar over it and under the title. Its button in the header hides it, or on a narrow page opens Jump to section.
- **⌘K** (Ctrl+K) on the Notes page jumps to a section of the page open: type a part of its title, Enter goes there.
- **Back to top** shows once the page is scrolled down, and under the outline.
- The pages inside a page are listed at its foot; the path above it opens the pages it is under.

## Icons

Pages' and groups' icons are SVG, never emoji, so they look the same on every system and follow the theme. The icon above a page's title opens the picker:

- **Library**: the icons of Lucide, Tabler and Phosphor, kept in the app (no network), searched by name and keyword. Favorites and the icons used lately come first; the libraries shown, the favorites and the recent ones are kept on the host (`~/.enowx-app/notes/icons.json`, `note.iconPrefs`, `note.iconPrefs.save`). An icon takes the text's color, which follows the theme, or one picked.
- **Custom**: the user's own SVG, pasted or from a file, cleaned before it is kept (`core/iconlib`): scripts, event handlers, links out, embedded images and foreign content go, its `viewBox` is set, and its colors become the text's unless kept. 24 KB at most once cleaned.

An icon is stored as `lib:name` (`lucide:rocket`), `#rrggbb` after it for a color, or `svg:<base64>` for the user's own. A page without one shows the default (`lucide:file-text`). Saving an emoji as an icon is refused with the icon to use instead.

Pages and groups of before had emoji: the daemon moves them to the closest SVG icon once as it starts, and a title's leading emoji into its page's icon. An emoji with none close gets the default, marked (`iconReview`) with a dot in the tree until the user picks one. Emoji in a page's text stay.

## Asking an agent

**Ask an agent** in the page's header opens a panel beside it to talk with an agent about the page: Claude Code, Codex or any other installed, picked per page. Suggestions start it on the common asks (tidy the page into to-dos, turn it into a plan, brainstorm it as a new project, summarize it), or ask anything.

- The agent works on notes only. It runs in an empty folder of its own (`~/.enowx-app/notes-chat/<note>`), out of the session list, with a guide of its own on writing to-do lists, plans, project ideas, meeting notes and tidy pages. Claude Code is held to it: only enowX's notes tools (list, read, create, write), web search, and none of its tools for files or commands. Other agents are asked to keep to it.
- Nothing it writes takes effect by itself. Each write is a proposal shown above the page, as the lines it changes (a new page in full); **Accept** makes it, **Reject** drops it, and **Undo** puts the page back after accepting. A proposal made before the page changed says so.
- Claude Code and Codex show as a chat; other agents as their terminal. The panel's **+** starts a new conversation.

The daemon keeps proposals in `~/.enowx-app/notes/proposals.json` (`note.proposals`, `note.accept`, `note.reject`, `notes.proposals`). A session started with `NotesFor` sets `ENOWX_NOTES_PROPOSE`, which makes `enowx-app notes write` and `new` propose, refuses `rm` and `move`, and leaves the MCP server with the notes tools alone.

## Groups

Groups keep pages together, like folders: a top-level page is in one group or none, and the pages under it go with it. **New group** (the folder button beside **New**) makes one, with a name, an icon and a color. In the tree each group is a header with its count of pages: a click folds it (every device sees it folded), **+** makes a page in it, and its menu edits or deletes it. Dragging a page onto a group's header puts it in the group, onto **No group** takes it out, and a page's menu moves it to any group; dragging a group onto another puts it before. Deleting a group keeps its pages, in no group. The folder button beside the filter shows one group's pages alone.

Groups are kept in `~/.enowx-app/notes/groups.json` and a page's group in its header (`group:`); `note.groups`, `note.group.save`, `note.group.delete`, `note.group.move` and `note.setGroup` change them, and `notes.groups` tells the apps.

## The pages

The tree beside the page lists them by group, filtered by title. Dragging a page onto another's upper or lower edge puts it before or after it, onto its middle under it. A page's menu adds a page inside it, moves it to the top level, or deletes it with the pages under it.

## Where they are kept

On the host, in `~/.enowx-app/notes`, a Markdown file each (`<id>.md`) with a header saying its title, icon, the page it is under and its place:

```
---
title: Release plan
icon: lucide:rocket
parent: 3fa94c1d2b7e
order: 0
created: 2026-10-25T14:02:11Z
updated: 2026-10-25T14:10:40Z
---

- [ ] tag the release
```

The daemon lists, reads, saves, moves and deletes them (`note.list`, `note.get`, `note.save`, `note.move`, `note.delete`) and tells every app when they change (`notes.changed`).

A team's pages, on a host signed in to the cloud as a member of it, are kept the same way in `~/.enowx-app/teams/<team>/notes`, apart from the user's own. Every method works on them alike, by their ID; `note.list` holds every space's, each page with its `team`, and a new top-level page goes in the space its `team` names (a page under another, in its parent's). The account's menu at the rail's foot picks which the Notes page shows ([cloud.md](cloud.md#personal-and-team)).

## Import and export

**Import…** in the menu beside **Notes** (or files dropped on the tree) brings notes in from other apps. The files are this device's: the app sends them to the host (a folder or several files as one .zip) and the host's daemon reads them, so it works the same on a server. What it reads, found out from the files:

| From | What to give it | What becomes of it |
|---|---|---|
| Notion | Its export (Markdown & CSV, or HTML), the .zip as Notion hands it out or unpacked | Pages nested as they were; a database a table, its rows with more than their properties pages under it (or every row a page, when asked); callouts quotes; links between pages `[[links]]`; images kept |
| Notion's API | An integration's token (notion.so/my-integrations), its pages shared with it | Every page and database it sees, the same way; the token goes to Notion for this import only, kept nowhere but in the vault when asked (a Notion credential), and a token of the vault's can be picked instead of typed |
| Obsidian, folders of Markdown | The vault or folder (.zip or not) | A folder at the top a group, a folder deeper a page holding its notes (the note of its name beside it, as Obsidian's folder notes); front matter's title, dates and tags; `[[wikilinks]]`, aliases, headings and embedded images kept |
| Markdown, text | A .md or .txt | A note |
| Evernote | An .enex (a notebook) | A note each in a group of the notebook's name, with to-dos, tags and attachments |
| Google Keep | Google Takeout's Keep folder | A note each, checklists to-dos, labels as tags, pictures kept; what is in Keep's trash skipped |
| HTML, Apple Notes | .html files (Apple Notes exports them through other apps) | A note each, a folder at the top a group |
| enowX | An export's .zip (folders) or a backup's .json | The notes as they were: IDs, groups with their icons and colors, order, times, files |

The dialog first shows what it would make (the notes, where they go, titles there already) and then imports them with their progress, which **Stop** takes back; a report says what was skipped and what had no form in enowX (an embed, an equation), kept as a link or text, or left out. HTML is read as data: nothing in it runs, and links keep only web and mail addresses. **Into the group** puts every note at the top in one group; otherwise each goes in the group its folder says.

With a token of the vault's, the import can be made again on its own, hourly, every 6 hours or daily, writing again what it made before: the app keeps when and which credential, reads the token from the vault each time (open, or its key remembered) and sends it to the host for that import alone. The dialog lists those imports, each to stop.

Importing the same source again finds the notes it made before (Notion's page IDs, the files' paths in a vault, …, kept in `notes/imports.json`): it skips them, writes them again, or makes copies, as picked.

**Export…** in a page's menu (with the pages under it), a group's menu, or **Export all…** writes them on the host and saves the file on this device:

- **Markdown**: one note as a .md with a header (title, icon, group, dates, its ID); several as a .zip of them.
- **Folders**: a .zip of folders of Markdown, a group's notes in its folder and a page's under it in one of its name, their images in `assets/`. Obsidian opens it as a vault and Notion imports it; **Import** makes the same notes again from it.
- **HTML**: one page with contents, its images in it.
- **PDF**: a page per note, its images in it (in the standard fonts, so what they lack, emoji, is left out).
- **Backup**: everything in one .json, images included, which **Import** restores on any host.

A note's images and files live beside the notes in `notes/assets/<its ID>/`, its body naming them `assets/<ID>/<file>`. One taken out of the page stays while undo, a kept version or another page can still name it; once nothing names it and it has been left alone for a week, the daemon removes it (it looks once a day), and the sync removes it on the other hosts.

The daemon's `note.import` (a job: `note.import.status`, `note.import.cancel`, `note.imports`, events `notes.import`) reads a file or folder of the host; `note.stage` puts the app's files there first. `note.export` writes an export where it's asked, or where `note.export.bytes` reads it for the app.

## Agents

Agents read and write the same notes with `enowx-app notes` ([cli.md](cli.md)) or the MCP server's `notes_list`, `notes_read`, `notes_create`, `notes_write` and `notes_delete` (icons with `icons_search`), as when the user asks one to keep a plan or a checklist there.

## Versions

Each page keeps its earlier versions on its host, in `notes/.history/<id>/`: what it was before an agent's change was accepted, before a sync from another host changed it, before a version was restored, and every 10 minutes while you write; the last 50. **Version history…** in a page's menu shows them and brings one back (`enowx-app notes history NOTE`, `notes restore NOTE VERSION`). They stay on their host: notes themselves are synced between hosts ([user-data-sync.md](user-data-sync.md)).

A long note needn't be read or written whole. `notes outline NOTE` (`notes_outline`) lists its sections: each heading with what follows it up to the next as high, and each titled part of a layout block (a tab, a toggle, a callout, a card), with an ID made of its title (`#canvas`, `#canvas-2` for a second) and its to-dos done. `notes show NOTE --section S` reads one, `notes write NOTE --section S --body -` (or `--append`) changes its content under its heading and keeps the rest, and `notes check NOTE --item TEXT` ticks the to-do with that text, or the only one with it in its text (`--undo` unticks). A Notes page agent's section writes and ticks are proposals too. The guide agents get explains the layout blocks (`core/agent.BlocksGuide`), so they can write them.

## Projects

A page attaches to any number of projects, on any host: a plan touching a frontend on this Mac and a backend on a server names both. The link is in the page's header (`projects:`), so it syncs with the page ([user-data-sync.md](user-data-sync.md)). A project with a Git remote is named by it (`git:github.com/me/app`), so its clones on other hosts are the same project; one without by its host and folder.

- **Attach to projects…** in a page's menu, or the badges' **+**, lists every host's projects (each host publishes them in the synced `catalog/`), to tick. Badges under the title say `name · host`, three and `+N`; a project whose host is out of reach reads *offline*, one gone from its host *broken*, a click attaching it again.
- The tree's project menu shows one project's pages, one host's, those of none, or all grouped by project (a page of several in each).
- In the session list, an open project lists its notes; its menu makes a new one attached to it, and a session's menu one for its project.
- Agents in a project's session are told `enowx-app notes --project .` lists its notes, and tick their to-dos as they finish them; a note made with `notes new` in a project's session is attached to that project.
- The Notes page's agent reads its page's projects read-only: `project_context` (stack, branch, last commit, folders, README) and `project_read` (a file), through the app when the project is on another host. Each context read is kept in `notes/context/`, synced, and answered when its host is out of reach. Its panel says which projects it has.

`enowx-app notes attach NOTE --project P [--host H]`, `notes detach NOTE (--project P | --all)`, `notes projects NOTE`, `notes --project P`, `project context P`, `project read P FILE`; MCP `notes_attach`, `notes_detach`, `notes_projects`, `notes_for_project`, `project_context`, `project_read`.

## Project Docs

A whole project (PRD, Technical Spec, ERD and the rest) documented as ordinary pages under one project note: [project-docs.md](project-docs.md).

## A note's agent from anywhere

A note keeps the agent its page talks to (`agent:` in its header, so the same on every host) and its conversations with it, in `notes/chats/<id>.json`: each turn the agent ends there is kept, so what was said on one host shows on the others, under the panel before a conversation starts. Other agents ask it with `enowx-app notes ask NOTE "…" [--wait]` (MCP `notes_ask`), read what was said with `notes chat NOTE` (`notes_chat`), and pick it with `notes agent NOTE --use codex` (`notes_agent`). What agents proposed is listed by `notes proposals`, and settled by `notes accept ID` or `notes reject ID` when the user asks; the note's own agent can't settle its own proposals.
