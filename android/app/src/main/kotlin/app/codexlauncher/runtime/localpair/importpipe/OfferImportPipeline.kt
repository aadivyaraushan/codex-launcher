package app.codexlauncher.runtime.localpair.importpipe

import app.codexlauncher.runtime.localpair.OfferRejectReason
import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import app.codexlauncher.runtime.localpair.TermuxOfferValidator
import app.codexlauncher.runtime.localpair.offer.PublicOfferJson

object OfferImportPipeline {
    data class Input(
        val offerJson: String,
        val nowUnix: Long,
        val waitingForOffer: Boolean,
        val providerAuthority: String,
        val providerPackage: String,
        val providerSignerSha256: String,
        val expectedTermuxSignerSha256: String,
        val seenOfferIds: Set<String>,
    )

    sealed class Outcome {
        data class Accepted(val offer: PublicLocalPairOffer) : Outcome()
        data class Rejected(val reason: OfferRejectReason) : Outcome()
        data object ParseFailed : Outcome()
    }

    fun evaluate(input: Input): Outcome {
        val parsed = PublicOfferJson.parse(input.offerJson)
        if (parsed.isFailure) return Outcome.ParseFailed
        val offer = parsed.getOrThrow()
        val reject =
            TermuxOfferValidator.validate(
                offer = offer,
                nowUnix = input.nowUnix,
                waitingForOffer = input.waitingForOffer,
                providerAuthority = input.providerAuthority,
                providerPackage = input.providerPackage,
                providerSignerSha256 = input.providerSignerSha256,
                expectedTermuxSignerSha256 = input.expectedTermuxSignerSha256,
                seenOfferIds = input.seenOfferIds,
            )
        if (reject != null) return Outcome.Rejected(reject)
        return Outcome.Accepted(offer)
    }
}
