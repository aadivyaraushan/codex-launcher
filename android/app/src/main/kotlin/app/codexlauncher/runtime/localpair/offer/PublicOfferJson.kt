package app.codexlauncher.runtime.localpair.offer

import app.codexlauncher.runtime.localpair.PublicLocalPairOffer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long

object PublicOfferJson {
    class SecretLeak : IllegalArgumentException("public local-pair offer must not contain secret")

    private val json = Json { ignoreUnknownKeys = false }

    fun parse(raw: String): Result<PublicLocalPairOffer> =
        runCatching {
            val element = json.parseToJsonElement(raw)
            val obj = element.jsonObject
            if (obj.containsKey("secret")) throw SecretLeak()
            fun requireString(key: String): String =
                obj[key]?.jsonPrimitive?.content ?: error("missing field $key")
            fun requireInt(key: String): Int =
                obj[key]?.jsonPrimitive?.int ?: error("missing field $key")
            fun requireLong(key: String): Long =
                obj[key]?.jsonPrimitive?.long ?: error("missing field $key")
            PublicLocalPairOffer(
                protocolVersion = requireInt("protocolVersion"),
                offerId = requireString("offerId"),
                expiresAtUnix = requireLong("expiresAt"),
                port = requireInt("port"),
                runtimeIdentity = requireString("runtimeIdentity"),
                tlsSpki = requireString("tlsSpki"),
                ephemeralPublicKey = requireString("ephemeralPublicKey"),
                challenge = requireString("challenge"),
            )
        }
}
