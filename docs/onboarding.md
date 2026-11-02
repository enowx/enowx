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

## How it's drawn

Onboarding paints from its own table of the four styles' values (`ui/onboardview/look.go`), not the theme's tokens, which are one set for the whole app. So each region can be partway from one style to another by itself, and every moment is a function of the clock (`scene.go`). The clock's stops are the design's, so a picture of a moment can be compared with the design's picture of the same moment (`TestPictures`, with `SHOTS` and `SHOTS_AT`).

Wires, ports and what flies are measured from what they join, each frame: an agent goes from its row's tile to its slot in the list, a host's wire from a port on the project's row to a port on the server's card.

## The change of style

Picking a style plays one change across the window: a wave, each region starting a beat after the last, its place, corners, colors and font crossing together, colors in Oklab. Words dip as the wave crosses them, and those on a fill take whichever ink reads on it by then. The wallpaper leaves first and comes in last. One ring moves between the style cards, and a style picked meanwhile follows. The app takes the style at once; only the drawing crosses. Glass is the design's: what's under a region blurred, more as its material comes, then tinted.

## Less motion

With Reduce Motion on (the system's, or the app's own), the opening holds the logo and name still for 2.2 seconds, and each stop is cut to.

## Differences from the prototype

What the prototype only pretends is real here, so its words differ: agents are found or not, as nothing says whether one is signed in; a saved host says Saved; the first task runs in a session of its own, which answers there rather than with a sample reply; the composer's model chip says the agent's default. Background offers the user's own image rather than a sample, and the panels' opacity and blur apply to that image, as in Settings. The prototype's style tour is jumped over: a change plays when one is picked.

## Links

The Community step's links are all in `ui/onboardview/links.go`. A link left empty hides its card. They open in the browser, and nothing claims the user followed or starred anything.
