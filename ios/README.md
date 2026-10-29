# Bordo for iOS

Native SwiftUI client for Bordo. It does what the web console does (chat with the agent, apps, regions, observe) from an iPhone.

## How it connects

The agent's port (7402) is deliberately firewalled on the server. The app opens an SSH session to the host
(ed25519/RSA key or password), then forwards a loopback port on the phone to `127.0.0.1:7402` on the server
through an SSH `direct-tcpip` channel. HTTP and the `/ws/chat` WebSocket then run over that tunnel exactly as
the web console's BFF does. Nothing new needs to be exposed on the server.

- Host keys are pinned on first use (fingerprint shown for approval); a changed key is refused.
- SSH key, passphrase, password and agent token are stored in the Keychain.
- The agent token (`BORDO_INTERNAL_TOKEN`) can be read from `<install dir>/deploy/bordo/.env` over SSH from the server form.
- You can generate an ed25519 key in the app and copy its public key into `~/.ssh/authorized_keys`.

## Build

```
brew install xcodegen
cd ios && xcodegen generate && open Bordo.xcodeproj
```
Set your signing team in Xcode to run on a device. Requires iOS 17+.

## Tests

`ParsingTests` run anywhere. `TunnelIntegrationTests` start an in-process SSH server in front of a real agent:

```
(cd agent && go build -o /tmp/bordo-agent .)
BORDO_AGENT_TOKEN=testtoken BORDO_CHAT_DB=/tmp/chats.db /tmp/bordo-agent --port 17402 &
TEST_RUNNER_BORDO_TEST_AGENT_PORT=17402 xcodebuild test -project Bordo.xcodeproj -scheme Bordo \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' CODE_SIGNING_ALLOWED=NO
```

## Not included yet

Env var editing, ECDSA keys, background connections (iOS suspends the SSH socket; the app reconnects on return).
