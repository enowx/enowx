# RAG: a project found by meaning

Agents find a project's code, docs, attached notes and memories by meaning, without reading it all. enowX indexes a project itself (`core/rag`), on the host that has the project, through its daemon. A session on a server works the same way.

## How it works

1. **Files:** git's tracked and untracked files, so `.gitignore` is honored; outside a repository, a walk that skips `node_modules`, `vendor`, `dist` and the like. Text files up to 500 KB.
2. **Never sent:** files that hold secrets by their name (`.env`, keys, certificates, credentials), and any chunk whose text holds a private key or a token.
3. **Chunks:** a file is cut at its top-level declarations (functions, types, classes, Markdown headings), each with the comments above it; small ones are packed together and long ones are cut at blank lines. A chunk is at most 1000 characters and keeps its lines and its symbol.
4. **Embedding:** each chunk is embedded with a header naming its file, lines and symbol.
5. **Incremental:** a chunk's ID is its file and its text's hash. Text that didn't change is never embedded again; when lines are added above it, it only moves.
6. **Search:** by meaning (cosine over the vectors) and by words (full-text search), fused by rank, so an exact name and a description both find their code.

## Consent first

Nothing is indexed until two things happen:

1. Settings → **RAG** names an embedder.
2. A project is turned on in the side panel's **Index** tool.

Turning a project on shows what would be sent before anything is (`rag.plan`):

- the embedder and its host;
- how many files and roughly how many tokens;
- what is never sent: secrets (`.env`, keys, files that look like credentials) and what `.gitignore` leaves out;
- a field for more paths to leave out.

**Agree and index** records consent for that embedder. If the embedder changes later, the project waits until the user agrees to the new one. Turning the project off deletes its index and keeps its memories.

## Settings → RAG

The host's settings (`settings/rag.json`) sync with the user's other data ([user-data-sync.md](user-data-sync.md)).

**Embedder**, one of:
- Voyage AI (`voyage-code-3` by default);
- any OpenAI-compatible `/v1/embeddings` endpoint (OpenAI, Ollama, OpenRouter);
- Text Embeddings Inference.

Each takes a model, dimensions, an address, and the key as a vault credential's name. The daemon asks the app's vault for the key when it embeds, so the key is never written to settings.

**Index kept**, one of:
- **SQLite on the host** (the default): one file in the daemon's folder, `rag/index.db`, with nothing else to run. Vectors are searched by brute force over a copy in memory: about 12 ms for 10,000 parts, 120 ms for 100,000.
- **PostgreSQL with pgvector:** a vault credential holds the connection URL (`postgres://user:password@host:5432/db`). enowX creates the `vector` extension when its user may, and keeps its table `enowx_rag_points`.

Vectors of two embedders never mix: a project's index is keyed by its ID and the embedder's.

**Test** embeds a line, connects to PostgreSQL when it is picked, and reports the vector's size.

## Hosts

Each host builds its own indexes: they are large and can be rebuilt, so they are never synced. Only the settings and memories sync. Hosts that share one PostgreSQL share the project's index.

## The Index tool

The side panel's **Code** group has an **Index** tool for the selected session's project. It shows:

- the index: how many parts, from how many files, when it was last indexed, and with which embedder;
- **Reindex**, and progress (`rag.progress`) while indexing runs;
- a search box. Results show file:line, the symbol and the matching part; clicking one opens the file at those lines, marked;
- the project's memories, which can be added, changed and deleted;
- **Give prompts the parts that bear on them**: at the start of a prompt, the four best results, up to 6000 characters, are added as context. This needs the start-of-prompt hook (Claude Code, Codex).
- **Offer to keep decisions as memories**: when a session ends, a dialog offers the agent's last answer to edit and save (`rag.offer`).
- **Turn off and delete the index**, which asks first.

## Staying fresh

The daemon indexes a project again:
- 15 seconds after a turn ends;
- after a commit it sees (the project's HEAD moved).

Either way it waits until no session in the project is working. Only changed files are embedded again: content hashes are compared, and unchanged parts are skipped. Memories and the notes attached to the project are indexed with it.

## For agents

The feature `rag` (Settings → Features, group Project) is off by default. When turned on, agents get its guide and these:

| CLI | MCP | |
|---|---|---|
| `rag search QUERY [--k N] [--scope code,docs,notes,memory]` | `rag_search` | file:line, symbol and the part found |
| `rag remember TEXT` | `rag_remember` | keeps a decision or gotcha |
| `rag memories`, `rag forget ID` | | lists and removes memories |
| `rag status`, `rag index` | `rag_status` | the index, and reindexing it |

A session reaches only its own project. Outside a session, the CLI uses the current folder, or `--project DIR`.

## Measured

Measured on this repository with `RAG_EVAL=<a Voyage key> RAG_EVAL_MODEL=voyage-4 go test ./core/rag -run TestEval`, SQLite store.

| | |
|---|---|
| First index | 1,441 files, 10,302 parts, about 2.25M tokens embedded, 1 min 19 s (four batches embedded at once) |
| Again, a few files changed | only their changed parts embedded, about 2 s |
| Again, nothing changed | nothing embedded, about 1 s |
| Search, the query's embedding included | p50 277 ms, p90 300 ms; the store's part is about 12 ms, the rest is the embedder's network |
| Recall@5, 20 questions about the code | 17/20 |
| Read per question | about 900 tokens of results, against whole files for a `git grep -l` of one word (hundreds of thousands of tokens on common words) |

**Recall.** The first run scored 14/20, but three of its expected files were wrong. For example, credentials are lent in `core/vault/lend.go`, not the daemon. The search had found the right file each time; with the expectations corrected, recall is 17/20.

**Misses.** In the three misses, docs that describe the feature ranked above its code. `--scope code` helps when only code is wanted.

The PostgreSQL store runs the same tests against pgvector: `ENOWX_RAG_TEST_POSTGRES=postgres://… go test ./core/rag`.
