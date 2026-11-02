# Contributor bot

Gives the **Contributor** role on the enowX Discord server to a member whose GitHub account has a merged pull request in the repository. It's a small service of its own, with no dependencies beyond Go's standard library: the enowX app doesn't ship or import it.

- `/connect` replies with a link only the member sees. They sign in with GitHub, and the two accounts are linked. With a merged pull request they get the role at once; without one they're told so, and get it when one merges.
- `/disconnect` removes the link and the role. The role otherwise stays.
- One GitHub account links to one Discord account.
- Only the Discord ID and the GitHub ID are stored (`LINKS_FILE`). The sign-in's token is used to read public data, then dropped.

## Set it up

**1. A Discord application** (Discord Developer Portal)
- Add a bot. Copy its token, and the application's ID and public key.
- Set **Interactions Endpoint URL** to `BASE_URL/discord/interactions`. Discord tests it when saved, so the service must be running first.
- Invite it with the scopes `bot` and `applications.commands` and the single permission **Manage Roles**.
- On the server, move the bot's role above `Contributor`, as a bot can only give roles below its own. It needs no privileged intents.

**2. A GitHub OAuth app** (GitHub, Settings, Developer settings)
- Set **Authorization callback URL** to `BASE_URL/github/callback`.
- Copy the client ID and generate a client secret. It asks for no scope, so it reads the member's public identity alone.

**3. A webhook on the repository** (Settings, Webhooks)
- **Payload URL** `BASE_URL/github/webhook`, content type `application/json`, a secret you generate, and the single event **Pull requests**.

## Configuration

Everything comes from the environment. Secrets are never in a file here.

| Variable | What it is |
|---|---|
| `BASE_URL` | Where the service is reached, over HTTPS |
| `PORT` | Optional, 8080 by default |
| `DISCORD_APP_ID`, `DISCORD_PUBLIC_KEY` | The application's |
| `DISCORD_BOT_TOKEN` | **Secret.** The bot's |
| `DISCORD_GUILD_ID` | The server: `1458149049902436578` |
| `DISCORD_ROLE_ID` | Contributor: `1566927925771636906` |
| `GITHUB_CLIENT_ID` | The OAuth app's |
| `GITHUB_CLIENT_SECRET` | **Secret.** The OAuth app's |
| `GITHUB_WEBHOOK_SECRET` | **Secret.** The webhook's |
| `STATE_SECRET` | **Secret.** Random, signs `/connect` links: `openssl rand -hex 32` |
| `GITHUB_REPO` | Only a merged pull request here counts: `enowdev/enowx` |
| `LINKS_FILE` | Where the links are kept, on a disk that lasts |

## Host it

Any small always-on host with HTTPS in front of it will do. It answers Discord within seconds, so it shouldn't sleep when idle.

```sh
docker build -t contributor-bot .
docker run -d --restart unless-stopped --env-file bot.env -v contributor-bot:/data -p 8080:8080 contributor-bot
```

Then register the slash commands once, on that server, and again if they change:

```sh
docker run --rm --env-file bot.env contributor-bot register
```

Without Docker: `go build`, then the same with the variables set. `GET /healthz` answers while it runs.

## Build and test

```sh
go build ./...
go test ./...
```

The tests use local stand-ins, and reach neither Discord nor GitHub.

## Known limits

- A delivery that fails to give the role answers an error on GitHub's side, where it can be delivered again from the webhook's page. Discord's rate limits aren't retried.
- The check runs on `/connect` and on merge, so a pull request merged while the service was down counts at the member's next `/connect`.
- A second GitHub account replaces a member's first. The role isn't reviewed then: it stays until `/disconnect`.
