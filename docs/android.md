# Running Android apps

The side panel's **Android** tool runs the session's app on an Android device and shows what it logs. The device may be on the session's host or on this machine: a project on a server runs on the laptop's emulator as well as on one of its own.

## Devices

The tool lists the devices of the session's host and, when that is a server, of this machine (the app connects to its own daemon beside the server's): emulators running, phones adb sees, and the virtual devices that can start (`android.status`, `core/android`).

- **Start** boots a virtual device: on this machine in the emulator's own window, on a server headless.
- **Stop** stops an emulator, or disconnects a phone connected over the network.
- **+ Phone:** on the phone, Developer options → Wireless debugging; pair once with its pairing address and code, then connect with its address (`adb pair`, `adb connect`). A phone on USB shows by itself; "allow USB debugging on the phone" until it is allowed.

The emulator runs on the host itself, accelerated by what the system offers: Hypervisor.framework on macOS, Windows Hypervisor Platform (or AEHD) on Windows, KVM on Linux. A host without it (most VPSes, which offer no nested virtualization) says so: the emulator runs there only very slowly, and the laptop's emulator or a phone is the way.

## Set up Android

**Set up Android** brings a host what building and running apps need (`enowx-app android setup`, `core/android/setup.go`): a JDK 17 (Eclipse Temurin, into `~/.enowx-app/jdk`) when none is 17 or newer, the SDK's newest command-line tools (as Google's repository lists them, checked against its SHA-1), the SDK's licenses, read and accepted by you, then platform-tools, the emulator, the newest platform, build-tools and a Google APIs system image for the host's architecture (arm64 on Apple Silicon and arm64 Linux, x86_64 elsewhere), and a virtual phone, `enowX_Phone`. The SDK goes where Android Studio puts it (`~/Library/Android/sdk`, `%LOCALAPPDATA%\Android\Sdk`, `~/Android/Sdk`), or where `ANDROID_HOME` says. On the session's host it runs in a tab of the session; on this machine in the tool, with a button for the licenses. A server's **Set up** page offers it too when this device has adb.

## Run app

The tool reads what the session's project is and the command that runs it on the device `$ANDROID_SERIAL` names, editable before it runs (`android.project`):

| Project | Command |
|---|---|
| Flutter | `flutter run -d "$ANDROID_SERIAL"` |
| Expo | `npx expo run:android --device "$ANDROID_SERIAL"` |
| React Native | `npx react-native run-android --deviceId "$ANDROID_SERIAL"` |
| Capacitor | `npx cap run android --target "$ANDROID_SERIAL"` |
| NativePHP Mobile | `php artisan native:run android` |
| Gradle (Kotlin, Java) | `./gradlew installDebug`, then the app launched |

**Run app** runs it in a new tab of the session, so hot reload's keys work, with the SDK on its `PATH`, `ANDROID_HOME`, the JDK enowX brought as `JAVA_HOME`, and `ANDROID_SERIAL`; for Expo and React Native, `adb reverse tcp:8081` first, so the app reaches Metro.

### A device on another host

When the device is on this machine and the session on a server, its tools reach the device through an **ADB bridge** (`android.adb.*`, `client.BridgeADB`): the server's daemon listens on a loopback port, hands each connection to the app, which carries it to a tunnel to this machine's ADB server; the tab gets `ADB_SERVER_SOCKET` pointing at that port, so `adb`, `flutter` and Gradle see the device as if it were plugged into the server. Metro's port is forwarded here, where `adb reverse` sends the phone.

## The screen

The tool's **Screen** and **Logcat** rows, under the device and its app, open one or the other in a tab of the session. **Show its screen** shows the device's screen there, to use: a click is a touch and a drag a swipe, typing types, right click is Back, and the buttons in its bar are Back, Home, Recent apps, the volume and Power. The device's host starts scrcpy's server on it ([scrcpy](https://github.com/Genymobile/scrcpy), Apache-2.0: `scrcpy-server` 5.0.1, downloaded once into `~/.enowx-app/tools` and checked against its SHA-256, pushed to the device), which sends the screen as H.264; the host carries it to the app (`android.mirror`, a tunnel), wherever the device is, and the app hands it to a web view on a loopback port of its own, whose page decodes it with WebCodecs. Touches and keys go back the same way (`android.mirror.input`). Closing the screen, or the app, stops the server.

**Pop out** (and a system whose web view can't decode video) opens it in scrcpy's own window instead, at full speed, with the clipboard: the scrcpy installed, or version 5.0.1 downloaded once the same way. For a device on a server, scrcpy talks to the server's adb through the bridge and its video comes through a port adb forwards there, tunneled here (`android.forward.open`, only ports adb lists).

## The log

The picked device's logcat, the app's lines only by default (by its package, from the project's files), by level, searchable (`android.logcat`). The daemon reads a device's log while someone looks at it, its last 5,000 lines.

## Catching a device's traffic

**Catch its traffic** sends the device's HTTP through the session's host's interceptor ([traffic.md](traffic.md)): a source there, `adb reverse` taking the device's `localhost` port to its host (forwarded on from this machine when the session is on a server), and its proxy setting pointing there (`android.proxy`). An emulator, rooted (`adb root`, which Google APIs images allow), gets enowX's CA among its user certificates; a phone gets it in Downloads, to install from Settings. Apps trust user certificates only when they say so, as a debug build's `network_security_config` can (`<debug-overrides>` with `<certificates src="user"/>`), and Flutter's HTTP client reads neither the proxy nor the system's certificates without a debug hook. **Stop catching** undoes the proxy setting.

## Not yet

- Redroid and other containers, devices of a third host, iOS.
- NativePHP's bundled PHP on x86_64 images isn't checked: on an x86_64 emulator, check its PHP runs there.

## Android TV

An Android TV is a device like a phone: **Connect a phone** connects one over the network (Developer options → Network debugging, then its address with `:5555`, no pairing), or a USB one shows by itself; its app runs, logs and has its traffic caught the same way. The list says **Android TV**, read from the device (`ro.build.characteristics`).

- **Its screen** fills the pane's width, with a remote under it: the D-pad, **OK** (the D-pad's center) and Menu, beside Back, Home and the volume above. In the screen, the keyboard's arrows move and Enter is OK.
- **A virtual TV:** **Make a virtual TV**, in the Connect a phone dialog, sets one up in a tab of the session (`enowx-app android setup --tv`): the newest Android TV system image for the host, and an AVD `enowX_TV` (1080p), beside the virtual phone.
- **Agents** press the remote's keys: `enowx-app android key up|down|left|right|ok|menu|play-pause`.
