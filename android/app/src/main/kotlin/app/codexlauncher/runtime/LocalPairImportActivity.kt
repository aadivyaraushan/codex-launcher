package app.codexlauncher.runtime

import android.app.Activity
import android.content.ComponentName
import android.net.Uri
import android.os.Bundle
import app.codexlauncher.diagnostics.AppLog
import app.codexlauncher.runtime.localpair.LocalPairAwaitingStore
import app.codexlauncher.runtime.localpair.LocalPairIntentGate
import app.codexlauncher.runtime.localpair.TermuxOfferValidator
import app.codexlauncher.runtime.localpair.handshake.LocalPairHandshake
import app.codexlauncher.runtime.localpair.importpipe.OfferImportPipeline
import java.util.concurrent.Executors

/**
 * Explicit Termux share receiver for public local-pair offers. Rejects
 * implicit launches and wrong MIME/URI shapes before any secret handling.
 */
class LocalPairImportActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val component = intent?.component?.flattenToShortString()
        val dataUri = intent?.data
        val uriCount =
            when {
                dataUri != null -> 1
                intent?.clipData != null -> intent!!.clipData!!.itemCount
                else -> 0
            }
        val reject =
            LocalPairIntentGate.reject(
                LocalPairIntentGate.Launch(
                    explicitComponent = component,
                    mimeType = intent?.type ?: TermuxOfferValidator.MIME,
                    uriCount = uriCount,
                    waitingForOffer = LocalPairAwaitingStore.waitingForOffer,
                ),
            )
        if (reject != null) {
            AppLog.info(
                feature = "local-pair",
                message = "local pair import rejected",
                fields = mapOf("reason" to reject::class.simpleName.orEmpty()),
            )
            finish()
            return
        }
        val uri = dataUri ?: intent?.clipData?.getItemAt(0)?.uri
        if (uri == null || !contentUriAuthorityAllowed(uri)) {
            AppLog.info(
                feature = "local-pair",
                message = "local pair offer uri rejected",
                fields = mapOf("reason" to "authority_or_missing"),
            )
            finish()
            return
        }
        val raw =
            runCatching {
                contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }
                    ?: error("empty offer stream")
            }.getOrElse { err ->
                AppLog.info(
                    feature = "local-pair",
                    message = "local pair offer read failed",
                    fields = mapOf("error" to (err.message ?: err::class.simpleName.orEmpty())),
                )
                finish()
                return
            }
        val outcome =
            OfferImportPipeline.evaluate(
                OfferImportPipeline.Input(
                    offerJson = raw,
                    nowUnix = System.currentTimeMillis() / 1000L,
                    waitingForOffer = LocalPairAwaitingStore.waitingForOffer,
                    providerAuthority = uri.authority.orEmpty(),
                    providerPackage = TermuxOfferValidator.REQUIRED_PACKAGE,
                    providerSignerSha256 = TermuxOfferValidator.PINNED_SIGNER_SHA256,
                    expectedTermuxSignerSha256 = TermuxOfferValidator.PINNED_SIGNER_SHA256,
                    seenOfferIds = LocalPairAwaitingStore.seenOfferIds,
                ),
            )
        when (outcome) {
            is OfferImportPipeline.Outcome.Accepted -> {
                LocalPairAwaitingStore.markSeen(outcome.offer.offerId)
                AppLog.info(
                    feature = "local-pair",
                    message = "local pair public offer accepted for attestation handshake",
                    fields =
                        mapOf(
                            "offer_id" to outcome.offer.offerId,
                            "port" to outcome.offer.port,
                            "component" to (component ?: ""),
                        ),
                )
                val appContext = applicationContext
                val accepted = outcome.offer
                Executors.newSingleThreadExecutor().execute {
                    val result = LocalPairHandshake.run(appContext, accepted)
                    if (result.acked) {
                        LocalPairAwaitingStore.clearWaiting()
                    }
                }
            }
            is OfferImportPipeline.Outcome.Rejected -> {
                AppLog.info(
                    feature = "local-pair",
                    message = "local pair offer pipeline rejected",
                    fields = mapOf("reason" to outcome.reason::class.simpleName.orEmpty()),
                )
            }
            OfferImportPipeline.Outcome.ParseFailed -> {
                AppLog.info(
                    feature = "local-pair",
                    message = "local pair offer parse rejected",
                    fields = mapOf("error" to "parse_failed"),
                )
            }
        }
        finish()
    }

    companion object {
        fun componentName(): ComponentName =
            ComponentName("app.codexlauncher", "app.codexlauncher.runtime.LocalPairImportActivity")

        fun contentUriAuthorityAllowed(uri: Uri): Boolean =
            uri.authority == TermuxOfferValidator.REQUIRED_AUTHORITY
    }
}
