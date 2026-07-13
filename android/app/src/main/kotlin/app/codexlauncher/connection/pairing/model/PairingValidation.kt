package app.codexlauncher.connection.pairing.model

import java.net.Inet6Address
import java.net.InetAddress
import java.util.Base64

internal object PairingValidation {
    fun isTailscaleAddress(host: String): Boolean {
        if (host.contains(':')) {
            if (host.contains('%')) return false
            val address = runCatching { InetAddress.getByName(host) }.getOrNull() as? Inet6Address ?: return false
            val bytes = address.address
            return bytes.size == 16 &&
                bytes[0] == 0xfd.toByte() &&
                bytes[1] == 0x7a.toByte() &&
                bytes[2] == 0x11.toByte() &&
                bytes[3] == 0x5c.toByte() &&
                bytes[4] == 0xa1.toByte() &&
                bytes[5] == 0xe0.toByte()
        }
        val parts = host.split('.')
        if (parts.size != 4) return false
        val octets =
            parts.map { part ->
                if (part.isEmpty() || part.length > 1 && part.startsWith('0') || part.any { !it.isDigit() }) return false
                part.toIntOrNull()?.takeIf { it in 0..255 } ?: return false
            }
        return octets[0] == 100 && octets[1] in 64..127
    }

    fun isSafeIdentifier(value: String): Boolean =
        value.length in 1..128 &&
            value.first().isAsciiLetterOrDigit() &&
            value.drop(1).all { it.isAsciiLetterOrDigit() || it in "._:-" }

    fun isSafeDeviceName(value: String): Boolean =
        value.isNotBlank() && value.encodeToByteArray().size <= 128 && value.none(Char::isISOControl)

    fun isCanonicalBase64Url(value: String, decodedBytes: Int): Boolean =
        runCatching {
            val decoded = Base64.getUrlDecoder().decode(value)
            decoded.size == decodedBytes && Base64.getUrlEncoder().withoutPadding().encodeToString(decoded) == value
        }.getOrDefault(false)

    private fun Char.isAsciiLetterOrDigit(): Boolean =
        this in 'A'..'Z' || this in 'a'..'z' || this in '0'..'9'
}
