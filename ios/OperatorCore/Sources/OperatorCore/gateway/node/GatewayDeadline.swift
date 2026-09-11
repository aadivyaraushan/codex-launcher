import Foundation

/// Bounding how long a node command may take.
///
/// The gateway hands every handler a `timeoutMilliseconds`, and most of them
/// were discarding it — which meant a caller had no way to bound a slow
/// connector, and a hung framework call or an unresponsive backend would wait
/// forever with nothing to say about it.
///
/// The clamp and the `TIMEOUT` failure code follow what
/// ForegroundWhatsAppComposeService already did, so every handler answers a
/// deadline the same way.
public enum GatewayDeadline {
    /// Used when the gateway supplies no deadline of its own.
    public static let defaultMilliseconds = 30_000
    /// A caller may ask for less than this, never more. A deadline a handler
    /// cannot be trusted to honour is not a deadline.
    public static let maximumMilliseconds = 30_000

    /// The deadline actually applied, given what the gateway asked for.
    public static func bounded(_ requested: Int?) -> Int {
        max(1, min(requested ?? Self.defaultMilliseconds, Self.maximumMilliseconds))
    }

    /// Runs `work` under a deadline, returning nil if the deadline passes
    /// first. The losing task is cancelled, though cancellation only stops
    /// work that checks for it — a synchronous framework call already in
    /// flight will still finish. What this guarantees is that the *caller*
    /// stops waiting, which is the part the gateway needs.
    public static func run<T: Sendable>(
        milliseconds: Int,
        _ work: @escaping @Sendable () async -> T) async -> T?
    {
        await withTaskGroup(of: T?.self) { group in
            group.addTask { await work() }
            group.addTask {
                try? await Task.sleep(nanoseconds: UInt64(max(1, milliseconds)) * 1_000_000)
                return nil
            }
            let first = await group.next() ?? nil
            group.cancelAll()
            return first
        }
    }
}
