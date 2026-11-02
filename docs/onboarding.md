# Onboarding

The first run, in the app (`ui/onboardview`). The app assembles itself: after an opening, every region of the window draws itself as a hairline frame, takes its material, and fills, while the user sets it up in a panel holding the side panel's place. Everything on screen follows from one clock, so a click moves the clock to the next stop, where it waits, and Back runs it in reverse.

## Who sees what

Every user goes through it. It can't be skipped, opening included, and optional steps keep Later. Nothing in it needs an agent or the network.

| Who | What they get |
|---|---|
| Nobody has done it here, and the host has no sessions or projects | The opening, then seven steps: Agents, Project, Host, Style, GOD of Agents, First task, Community |
| Not done yet, but the host has sessions or projects (a returning user) | The opening, then the frames, then their settings filled in to confirm or change, then Community |
| Done on this account, new to this device | The frames, then this device's own steps: Agents, Host |
| Settings, App, Run setup again | All of it, from the opening |

The account's record is `settings/onboarding.json` on the host, which syncs with the user's data; the device's is `onboardedDevice` in `app.json`. A host too old to answer leaves the device's record to go by.

## The change of style

Picking a style plays one change across the window: a wave, each region starting a beat after the last, its place, corners and color crossing together, colors in Oklab. One ring moves between the style cards. A style picked meanwhile follows.

The app's theme is one set of tokens, so a widget can't be in two styles at once. So what's in the regions fades out in the old style (0.12 seconds), the style changes, and it fades back in as the wave passes (about half a second), while the surfaces carry the change. That is a change from the design, where text crosses region by region.

## Less motion

With Reduce Motion on (the system's, or the app's own), the opening holds the logo and name still for 1.6 seconds, and each stop is cut to.

## Links

The Community step's links are all in `ui/onboardview/links.go`. A link left empty hides its card, as Discord's is now. They open in the browser, and nothing claims the user followed or starred anything.
