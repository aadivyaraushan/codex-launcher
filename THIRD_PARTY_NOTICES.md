# Third-party notices

Codex Launcher is Apache-2.0 licensed. It also depends on third-party software
under its own licenses. The release SPDX SBOMs are the source of truth for the
exact dependency versions in each artifact.

Direct runtime dependency families include:

| Project | Used by | License family |
| --- | --- | --- |
| AndroidX and Jetpack Compose | Android launcher | Apache-2.0 |
| CameraX | Android QR scanner | Apache-2.0 |
| Kotlin coroutines and serialization | Android launcher | Apache-2.0 |
| Moonshine Voice | Android on-device dictation | MIT |
| ONNX Runtime | Moonshine model execution on Android | MIT |
| OkHttp | Android pinned TLS and WebSocket transport | Apache-2.0 |
| Tink | Android local cryptography | Apache-2.0 |
| ZXing Core | Android QR decoding | Apache-2.0 |
| coder/websocket | Companion WebSocket transport | ISC |
| golang.org/x/sys | Companion operating-system integration | BSD-3-Clause |
| modernc.org/sqlite | Companion durable local state | BSD-3-Clause |

Instrument Sans and JetBrains Mono are distributed under the SIL Open Font
License 1.1. See [NOTICE](NOTICE) for their copyright notices.

This summary is not a replacement for the license files and metadata shipped by
each dependency. Review the SPDX SBOM beside the APK/AAB or desktop archives for
the complete resolved dependency inventory.
