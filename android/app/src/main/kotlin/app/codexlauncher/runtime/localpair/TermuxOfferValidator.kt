package app.codexlauncher.runtime.localpair

/**
 * Validates a Termux-shared public local-pair offer before Operator connects.
 * The pairing secret is never present in this payload.
 */
data class PublicLocalPairOffer(
    val protocolVersion: Int,
    val offerId: String,
    val expiresAtUnix: Long,
    val port: Int,
    val runtimeIdentity: String,
    val tlsSpki: String,
    val ephemeralPublicKey: String,
    val challenge: String,
)

sealed class OfferRejectReason {
    data object WrongPort : OfferRejectReason()
    data object WrongProtocol : OfferRejectReason()
    data object Expired : OfferRejectReason()
    data object WrongProvider : OfferRejectReason()
    data object WrongSigner : OfferRejectReason()
    data object SetupNotWaiting : OfferRejectReason()
    data object Replay : OfferRejectReason()
}

object TermuxOfferValidator {
    const val REQUIRED_PORT = 9443
    const val REQUIRED_PROTOCOL = 1
    const val REQUIRED_AUTHORITY = "com.termux.sharedfile"
    const val REQUIRED_PACKAGE = "com.termux"
    const val MIME = "application/vnd.app.codexlauncher.local-pair+json"
    const val PINNED_SIGNER_SHA256 = "738f0a30a04d3c8a1be304af18d0779bcf3ea88fb60808f657a3521861c2ebf9"

    fun validate(
        offer: PublicLocalPairOffer,
        nowUnix: Long,
        waitingForOffer: Boolean,
        providerAuthority: String,
        providerPackage: String,
        providerSignerSha256: String,
        expectedTermuxSignerSha256: String,
        seenOfferIds: Set<String>,
    ): OfferRejectReason? {
        if (!waitingForOffer) return OfferRejectReason.SetupNotWaiting
        if (providerAuthority != REQUIRED_AUTHORITY || providerPackage != REQUIRED_PACKAGE) {
            return OfferRejectReason.WrongProvider
        }
        if (!providerSignerSha256.equals(expectedTermuxSignerSha256, ignoreCase = true)) {
            return OfferRejectReason.WrongSigner
        }
        if (offer.protocolVersion != REQUIRED_PROTOCOL) return OfferRejectReason.WrongProtocol
        if (offer.port != REQUIRED_PORT) return OfferRejectReason.WrongPort
        if (nowUnix > offer.expiresAtUnix) return OfferRejectReason.Expired
        if (offer.offerId in seenOfferIds) return OfferRejectReason.Replay
        return null
    }
}
