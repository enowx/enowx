# Updates

The app updates itself (`ui/updates.go`), with MyGo's updater: it downloads the new version's signed archive, or a delta from the running version, checks the signature against the public key built into the app, and replaces the app.

## In the app

- A little after it starts (15 s) and every 6 hours, the app asks for a newer version, unless **Check for updates automatically** is off in the update dialog.
- A version found shows in a card at the window's bottom right: **Update now** installs it there, **What's new** opens the dialog, **Later** puts the card off until the app starts again (each launch shows it again, until the version is installed or skipped). It also stays as **Update to x.y.z** in the title bar.
- The chip opens the dialog: the release notes, then **Install update**, **Later** or **Skip this version** (a skipped version shows again only when asked for). Installing shows its progress and can be cancelled; the running app goes on meanwhile.
- **Restart now** runs the new version. Sessions keep running: their programs belong to the daemon, which outlives the app.
- **The daemon updates only when that stops nothing.** As the app connects it asks the daemon what a restart would stop now (`daemon.restartPlan`): agents at work or asking, and what runs in terminals. With nothing, the app replaces it, as before, and idle agents come back on their conversations. Otherwise the daemon stays at its version and a card says what would stop: **Restart it now** (it asks first) or **When agents are done**, which replaces it once the plan is empty. Ignored, the card changes nothing, and the next connection looks again. Servers work the same way, by the daemon they already run. A daemon too old to say (0.6.2 or before) is judged by its sessions, every terminal counting, as it can't say what runs there. See [daemon.md](daemon.md#when-a-daemon-restarts).
- **Check for updates** in the command palette asks now and says how it went.

Development builds (`make dev`) and builds without an update feed don't update themselves: the palette says so, and the background checks stay quiet.

## Where updates come from

GitHub Releases of `enowdev/enowx` (`updates.github` in `cmd/enowx/mygo.json`): apps read the update manifests of the latest published release, and check each update's signature against `updates.publicKey`, built into them.

The secret half of that key signs every release. It is the repository secret `MYGO_UPDATER_PRIVATE_KEY`, and a copy is kept by the maintainer outside the repository: installed apps accept only updates signed with it, so losing it strands them, and whoever has it can ship code to every user. Never commit it.

### Two versions at once

An app older than the daemon it reaches leaves the daemon as it is and says so in a card ("This enowX is older than enowxd…"): only an app at least as new as its daemon replaces it, so an app the update left running can't take the daemon back to its version, nor the new app take it again, over and over (issue #49). On its version's first run on a device, the app ends the other apps of its program still running, asking each to quit and ending it after 5 seconds: their daemon, hooks and command line, run with a word after the program's name, are left alone.

## Releasing

1. Set the version in `cmd/enowx/mygo.json`, and add its section to `CHANGELOG.md` (`## x.y.z`): it is what the update dialog shows.
2. Commit, then tag and push: `git tag vx.y.z && git push origin vx.y.z`.
3. The Release workflow (`.github/workflows/release.yml`) checks the tag against the version and the changelog, creates a draft release, and builds every platform on its own runner (`mygo build -upload`): the installers, each platform's signed archive, delta updates from the last versions, and `update-<platform>.json`, uploaded to the draft.
4. Look at the draft, then publish it: from then on, apps find it within six hours, or at once with **Check for updates**. Publish it as the latest and not as a pre-release: the feed only reads the latest. Publishing also posts the version's changelog to #releases on the Discord server, mentioning Update Notification (the `announce` job, as the bot, with the `DISCORD_RELEASE_BOT_TOKEN` secret: Discord lets only a bot with Mention Everyone ping a role that isn't mentionable). A failed post doesn't touch the release, and a rerun doesn't post twice.

Running the workflow by hand builds the same files as the run's artifacts, publishing nothing.

Apps installed by a package manager (the `.deb` in `/opt`) are updated by it, not by themselves; the disk image's app, the Windows setup's and Linux's `install.sh` install update themselves.
