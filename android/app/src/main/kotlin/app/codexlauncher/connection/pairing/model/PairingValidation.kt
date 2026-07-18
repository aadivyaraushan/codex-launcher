package app.codexlauncher.connection.pairing.model

import app.codexlauncher.connection.security.publicaddress.PublicInetAddress
import java.net.InetAddress
import java.util.Base64

internal object PairingValidation {
    fun isSafePublicEndpoint(host: String): Boolean {
        if (host.isEmpty()) return false
        if (host.contains(':')) return isSafePublicIpv6(host)
        return isSafePublicIpv4OrHostname(host)
    }

    private fun isSafePublicIpv6(host: String): Boolean {
        if (host.contains('%')) return false
        val address = runCatching { InetAddress.getByName(host) }.getOrNull() ?: return false
        return PublicInetAddress.isPublic(address)
    }

    private fun isSafePublicIpv4OrHostname(host: String): Boolean {
        val octets = strictDottedDecimalOctets(host)
        return if (octets != null) {
            PublicInetAddress.isPublicIpv4(octets[0], octets[1], octets[2], octets[3])
        } else {
            isSafeHostname(host)
        }
    }

    private fun strictDottedDecimalOctets(host: String): IntArray? {
        val parts = host.split('.')
        if (parts.size != 4) return null
        val octets = IntArray(4)
        parts.forEachIndexed { index, part ->
            if (part.isEmpty()) return null
            if (!part.all { it in '0'..'9' }) return null
            if (part.length > 1 && part[0] == '0') return null
            val value = part.toIntOrNull() ?: return null
            if (value !in 0..255) return null
            octets[index] = value
        }
        return octets
    }

    private fun isSafeHostname(host: String): Boolean {
        if (host.length !in 1..253) return false
        val labels = host.split('.')
        if (labels.size < 2) return false
        if (labels.any { label -> !isSafeLabel(label) }) return false
        val tld = labels.last()
        if (tld.none { it in 'a'..'z' || it in 'A'..'Z' }) return false
        return true
    }

    private fun isSafeLabel(label: String): Boolean {
        if (label.length !in 1..63) return false
        if (label.any { it !in 'a'..'z' && it !in 'A'..'Z' && it !in '0'..'9' && it != '-' }) return false
        if (label.first() == '-' || label.last() == '-') return false
        return true
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
