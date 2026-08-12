# Phase 9 — Soft boot reconfirm (Pixel)

**Timestamp (UTC):** 2026-08-12T23:17:49Z to 2026-08-12T23:21:12Z
**Serial:** 4B230DLAQ001Z5
**Mode:** soft service stop/restore (no full Android reboot)
**Result:** PASS (bridge :9443 + gateway :18789 live after restore)

Companion to phase9-boot-persistence-pixel.md (earlier soft cycle at 23:11:41Z). This pass reconfirmed after an incomplete overnight stop left gateway down.

## Procedure

1. Before: operator-phone-boot status bridge=up gateway=up; health taskCapable=true localPair=acked; ports 18789/9443 open.
2. Stop: sv force-stop + kill stubborn operator-phone-runtime / openclaw binaries. Gateway down (18789 closed).
3. Restore: operator-phone-boot ensure (watchdog already running) + sv up gateway then runtime. Gateway cold-start ~100s (GATEWAY_PORT_UP try=53). Bounce runtime after :18789 listen so turnproxy connects.
4. After: bridge=up gateway=up; ports open; health taskCapable=true localPair=acked; log turn proxy connected at 23:21:12Z.

## Evidence

- phase9-softboot-20260812T231749Z.txt
- phase9-softboot-final-20260812T231749Z.txt
- phase9-softboot-after-20260812T231749Z.txt
- bridge-health-phase9-softboot.json
- phase9-softboot-TS.txt

## Notes

- Full device reboot UI proof still not run (USB stranding risk overnight).
- ensure alone does not revive stopped children while runsvdir holds the lock; explicit sv up required.
- Money-safe probes only.
