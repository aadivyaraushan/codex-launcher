package app.codexlauncher.capability.handoff

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class HandOffActionsTest {
    @Test
    fun knownHandOffAppsResolveToAndroidPackages() {
        assertEquals("com.instagram.android", HandOffActions.androidPackage("Instagram"))
        assertEquals("com.instagram.android", HandOffActions.androidPackage("instagram"))
        assertEquals("com.venmo", HandOffActions.androidPackage("Venmo"))
        assertEquals("com.squareup.cash", HandOffActions.androidPackage("Cash App"))
        assertEquals("com.zellepay.zelle", HandOffActions.androidPackage("Zelle"))
        assertEquals("com.starbucks.mobilecard", HandOffActions.androidPackage("Starbucks"))
        assertEquals("com.chipotle.ordering", HandOffActions.androidPackage("Chipotle"))
        assertEquals("com.spotify.music", HandOffActions.androidPackage("Spotify"))
        assertEquals("com.spotify.music", HandOffActions.androidPackage("spotify"))
        assertEquals("com.audible.application", HandOffActions.androidPackage("Audible"))
        assertEquals("com.audible.application", HandOffActions.androidPackage("audible"))
        assertEquals("com.apple.android.music", HandOffActions.androidPackage("Apple Music"))
        assertEquals("com.apple.android.music", HandOffActions.androidPackage("apple music"))
        assertEquals("com.google.android.apps.messaging", HandOffActions.androidPackage("Messages"))
        assertEquals("com.google.android.apps.messaging", HandOffActions.androidPackage("messages"))
        assertEquals("com.discord", HandOffActions.androidPackage("Discord"))
        assertEquals("com.discord", HandOffActions.androidPackage("discord"))
        assertEquals("com.ubercab", HandOffActions.androidPackage("Uber"))
        assertEquals("com.ubercab", HandOffActions.androidPackage("uber"))
        assertEquals("com.ubercab.eats", HandOffActions.androidPackage("Uber Eats"))
        assertEquals("com.ubercab.eats", HandOffActions.androidPackage("uber eats"))
        assertEquals("com.resy.android.prod", HandOffActions.androidPackage("Resy"))
        assertEquals("com.resy.android.prod", HandOffActions.androidPackage("resy"))
        assertEquals("com.dd.doordash", HandOffActions.androidPackage("DoorDash"))
        assertEquals("com.dd.doordash", HandOffActions.androidPackage("doordash"))
        assertEquals("com.google.android.apps.photos", HandOffActions.androidPackage("Google Photos"))
        assertEquals("com.google.android.apps.photos", HandOffActions.androidPackage("google photos"))
        assertEquals("com.microsoft.teams", HandOffActions.androidPackage("Teams"))
        assertEquals("com.microsoft.teams", HandOffActions.androidPackage("teams"))
        assertEquals("com.booking", HandOffActions.androidPackage("Booking.com"))
        assertEquals("com.booking", HandOffActions.androidPackage("booking.com"))
        assertEquals("com.tripadvisor.tripadvisor", HandOffActions.androidPackage("Tripadvisor"))
        assertEquals("com.tripadvisor.tripadvisor", HandOffActions.androidPackage("tripadvisor"))
        assertEquals("com.viator.mobile.android", HandOffActions.androidPackage("Viator"))
        assertEquals("com.viator.mobile.android", HandOffActions.androidPackage("viator"))
        assertEquals("com.stubhub", HandOffActions.androidPackage("StubHub"))
        assertEquals("com.stubhub", HandOffActions.androidPackage("stubhub"))
        assertEquals("com.alltrails.alltrails", HandOffActions.androidPackage("AllTrails"))
        assertEquals("com.alltrails.alltrails", HandOffActions.androidPackage("alltrails"))
        assertEquals("com.taskrabbit.droid.consumer", HandOffActions.androidPackage("Taskrabbit"))
        assertEquals("com.taskrabbit.droid.consumer", HandOffActions.androidPackage("taskrabbit"))
        assertEquals("com.thumbtack.consumer", HandOffActions.androidPackage("Thumbtack"))
        assertEquals("com.thumbtack.consumer", HandOffActions.androidPackage("thumbtack"))
        assertEquals("com.creditkarma.mobile", HandOffActions.androidPackage("Credit Karma"))
        assertEquals("com.creditkarma.mobile", HandOffActions.androidPackage("credit karma"))
        assertEquals("com.intuit.turbotax.mobile", HandOffActions.androidPackage("TurboTax"))
        assertEquals("com.intuit.turbotax.mobile", HandOffActions.androidPackage("turbotax"))
        assertEquals("me.lyft.android", HandOffActions.androidPackage("Lyft"))
        assertEquals("me.lyft.android", HandOffActions.androidPackage("lyft"))
        assertEquals("com.google.android.keep", HandOffActions.androidPackage("Google Keep"))
        assertEquals("com.google.android.keep", HandOffActions.androidPackage("google keep"))
        assertEquals("com.whatsapp", HandOffActions.androidPackage("WhatsApp"))
        assertEquals("com.whatsapp", HandOffActions.androidPackage("whatsapp"))
        assertEquals("com.facebook.orca", HandOffActions.androidPackage("Messenger"))
        assertEquals("com.facebook.orca", HandOffActions.androidPackage("messenger"))
        assertEquals("org.thoughtcrime.securesms", HandOffActions.androidPackage("Signal"))
        assertEquals("org.thoughtcrime.securesms", HandOffActions.androidPackage("signal"))
        assertEquals("com.google.android.apps.maps", HandOffActions.androidPackage("Google Maps"))
        assertEquals("com.google.android.apps.maps", HandOffActions.androidPackage("google maps"))
        assertEquals("com.netflix.mediaclient", HandOffActions.androidPackage("Netflix"))
        assertEquals("com.netflix.mediaclient", HandOffActions.androidPackage("netflix"))
        assertEquals("com.facebook.katana", HandOffActions.androidPackage("Facebook"))
        assertEquals("com.facebook.katana", HandOffActions.androidPackage("facebook"))
        assertEquals("com.united.mobile.android", HandOffActions.androidPackage("United"))
        assertEquals("com.united.mobile.android", HandOffActions.androidPackage("united"))
        assertEquals("com.delta.mobile.android", HandOffActions.androidPackage("Delta"))
        assertEquals("com.delta.mobile.android", HandOffActions.androidPackage("delta"))
        assertEquals("com.southwestairlines.mobile", HandOffActions.androidPackage("Southwest"))
        assertEquals("com.southwestairlines.mobile", HandOffActions.androidPackage("southwest"))
        assertEquals("com.aa.android", HandOffActions.androidPackage("American Airlines"))
        assertEquals("com.aa.android", HandOffActions.androidPackage("american airlines"))
        assertEquals("com.citymapper.app.release", HandOffActions.androidPackage("Citymapper"))
        assertEquals("com.citymapper.app.release", HandOffActions.androidPackage("citymapper"))
        assertEquals("com.google.android.youtube", HandOffActions.androidPackage("YouTube"))
        assertEquals("com.google.android.youtube", HandOffActions.androidPackage("youtube"))
        assertEquals("com.airbnb.android", HandOffActions.androidPackage("Airbnb"))
        assertEquals("com.airbnb.android", HandOffActions.androidPackage("airbnb"))
        assertEquals("com.opentable", HandOffActions.androidPackage("OpenTable"))
        assertEquals("com.opentable", HandOffActions.androidPackage("opentable"))
        assertEquals("com.grubhub.android", HandOffActions.androidPackage("Grubhub"))
        assertEquals("com.grubhub.android", HandOffActions.androidPackage("grubhub"))
        assertEquals("com.instagram.barcelona", HandOffActions.androidPackage("Threads"))
        assertEquals("com.instagram.barcelona", HandOffActions.androidPackage("threads"))
        assertEquals("com.zhiliaoapp.musically", HandOffActions.androidPackage("TikTok"))
        assertEquals("com.zhiliaoapp.musically", HandOffActions.androidPackage("tiktok"))
        assertEquals("com.expedia.bookings", HandOffActions.androidPackage("Expedia"))
        assertEquals("com.expedia.bookings", HandOffActions.androidPackage("expedia"))
        assertEquals("com.target.ui", HandOffActions.androidPackage("Target"))
        assertEquals("com.target.ui", HandOffActions.androidPackage("target"))
        assertEquals("com.walmart.android", HandOffActions.androidPackage("Walmart"))
        assertEquals("com.walmart.android", HandOffActions.androidPackage("walmart"))
        assertEquals("com.nike.omega", HandOffActions.androidPackage("Nike"))
        assertEquals("com.nike.omega", HandOffActions.androidPackage("nike"))
        assertEquals("com.sephora", HandOffActions.androidPackage("Sephora"))
        assertEquals("com.sephora", HandOffActions.androidPackage("sephora"))
        assertEquals("com.wayfair.wayfair", HandOffActions.androidPackage("Wayfair"))
        assertEquals("com.wayfair.wayfair", HandOffActions.androidPackage("wayfair"))
        assertEquals("com.kayak.android", HandOffActions.androidPackage("Kayak"))
        assertEquals("com.kayak.android", HandOffActions.androidPackage("kayak"))
        // Callers: ./android/gradlew :app:testDebugUnitTest --tests HandOffActionsTest
        // User ask: HandOffActions for Priceline / LinkedIn / eBay (Wave1Specs 51 → 54).
        assertEquals("com.priceline.android.negotiator", HandOffActions.androidPackage("Priceline"))
        assertEquals("com.priceline.android.negotiator", HandOffActions.androidPackage("priceline"))
        assertEquals("com.linkedin.android", HandOffActions.androidPackage("LinkedIn"))
        assertEquals("com.linkedin.android", HandOffActions.androidPackage("linkedin"))
        assertEquals("com.ebay.mobile", HandOffActions.androidPackage("eBay"))
        assertEquals("com.ebay.mobile", HandOffActions.androidPackage("ebay"))
        // Callers: ./android/gradlew :app:testDebugUnitTest --tests HandOffActionsTest
        // User ask: HandOffActions for Pinterest (Wave1Specs 54 → 55).
        assertEquals("com.pinterest", HandOffActions.androidPackage("Pinterest"))
        assertEquals("com.pinterest", HandOffActions.androidPackage("pinterest"))
        // Callers: Wave1Specs → HandOffActions, stage1, deeplink_proof, this test.
        // User ask: HandOffActions for Duolingo/Fitbit/Shazam (Wave1Specs 55 → 58).
        assertEquals("com.duolingo", HandOffActions.androidPackage("Duolingo"))
        assertEquals("com.duolingo", HandOffActions.androidPackage("duolingo"))
        assertEquals("com.fitbit.FitbitMobile", HandOffActions.androidPackage("Fitbit"))
        assertEquals("com.fitbit.FitbitMobile", HandOffActions.androidPackage("fitbit"))
        assertEquals("com.shazam.android", HandOffActions.androidPackage("Shazam"))
        assertEquals("com.shazam.android", HandOffActions.androidPackage("shazam"))
        // Callers: Wave1Specs → HandOffActions, stage1, deeplink_proof, this test.
        // User ask: HandOffActions for Chromecast/YouTube Music/SoundCloud (Wave1Specs 58 → 61).
        assertEquals("com.google.android.apps.chromecast.app", HandOffActions.androidPackage("Chromecast"))
        assertEquals("com.google.android.apps.chromecast.app", HandOffActions.androidPackage("chromecast"))
        assertEquals("com.google.android.apps.youtube.music", HandOffActions.androidPackage("YouTube Music"))
        assertEquals("com.google.android.apps.youtube.music", HandOffActions.androidPackage("youtube music"))
        assertEquals("com.google.android.apps.youtube.music", HandOffActions.androidPackage("youtubemusic"))
        assertEquals("com.soundcloud.android", HandOffActions.androidPackage("SoundCloud"))
        assertEquals("com.soundcloud.android", HandOffActions.androidPackage("soundcloud"))
        // Callers: Wave1Specs → HandOffActions, stage1, deeplink_proof, this test.
        // User ask: HandOffActions for Pandora/Asana/Trello (Wave1Specs 61 → 64).
        assertEquals("com.pandora.android", HandOffActions.androidPackage("Pandora"))
        assertEquals("com.pandora.android", HandOffActions.androidPackage("pandora"))
        assertEquals("com.asana.app", HandOffActions.androidPackage("Asana"))
        assertEquals("com.asana.app", HandOffActions.androidPackage("asana"))
        assertEquals("com.trello", HandOffActions.androidPackage("Trello"))
        assertEquals("com.trello", HandOffActions.androidPackage("trello"))
        // Callers: HandOffActionsTest; User ask: Wave1Specs 64→67 HandOffActions for mstodo/googledocs/dropbox.
        assertEquals("com.microsoft.todos", HandOffActions.androidPackage("Microsoft To Do"))
        assertEquals("com.microsoft.todos", HandOffActions.androidPackage("microsoft to do"))
        assertEquals("com.microsoft.todos", HandOffActions.androidPackage("mstodo"))
        assertEquals("com.google.android.apps.docs.editors.docs", HandOffActions.androidPackage("Google Docs"))
        assertEquals("com.google.android.apps.docs.editors.docs", HandOffActions.androidPackage("google docs"))
        assertEquals("com.google.android.apps.docs.editors.docs", HandOffActions.androidPackage("googledocs"))
        assertEquals("com.dropbox.android", HandOffActions.androidPackage("Dropbox"))
        assertEquals("com.dropbox.android", HandOffActions.androidPackage("dropbox"))
        // Callers: HandOffActionsTest; User ask: Wave1Specs 67→70 HandOffActions for googlesheets/evernote/googleslides.
        assertEquals("com.google.android.apps.docs.editors.sheets", HandOffActions.androidPackage("Google Sheets"))
        assertEquals("com.google.android.apps.docs.editors.sheets", HandOffActions.androidPackage("google sheets"))
        assertEquals("com.google.android.apps.docs.editors.sheets", HandOffActions.androidPackage("googlesheets"))
        assertEquals("com.evernote", HandOffActions.androidPackage("Evernote"))
        assertEquals("com.evernote", HandOffActions.androidPackage("evernote"))
        assertEquals("com.google.android.apps.docs.editors.slides", HandOffActions.androidPackage("Google Slides"))
        assertEquals("com.google.android.apps.docs.editors.slides", HandOffActions.androidPackage("google slides"))
        assertEquals("com.google.android.apps.docs.editors.slides", HandOffActions.androidPackage("googleslides"))
        // Callers: HandOffActionsTest; User ask: Wave1Specs 70→73 HandOffActions for pocketcasts/goodreads/kindle.
        assertEquals("au.com.shiftyjelly.pocketcasts", HandOffActions.androidPackage("Pocket Casts"))
        assertEquals("au.com.shiftyjelly.pocketcasts", HandOffActions.androidPackage("pocket casts"))
        assertEquals("au.com.shiftyjelly.pocketcasts", HandOffActions.androidPackage("pocketcasts"))
        assertEquals("com.goodreads", HandOffActions.androidPackage("Goodreads"))
        assertEquals("com.goodreads", HandOffActions.androidPackage("goodreads"))
        assertEquals("com.amazon.kindle", HandOffActions.androidPackage("Kindle"))
        assertEquals("com.amazon.kindle", HandOffActions.androidPackage("kindle"))
        // Callers: HandOffActionsTest; User ask: Wave1Specs 73→76 HandOffActions for claude/chatgpt/grok.
        assertEquals("com.anthropic.claude", HandOffActions.androidPackage("Claude"))
        assertEquals("com.anthropic.claude", HandOffActions.androidPackage("claude"))
        assertEquals("com.openai.chatgpt", HandOffActions.androidPackage("ChatGPT"))
        assertEquals("com.openai.chatgpt", HandOffActions.androidPackage("chatgpt"))
        assertEquals("com.openai.chatgpt", HandOffActions.androidPackage("chat gpt"))
        assertEquals("ai.x.grok", HandOffActions.androidPackage("Grok"))
        assertEquals("ai.x.grok", HandOffActions.androidPackage("grok"))
    }

    @Test
    fun unknownAppsDoNotGuessAPackage() {
        assertNull(HandOffActions.androidPackage("SomeOtherApp"))
        assertNull(HandOffActions.androidPackage(""))
    }

    @Test
    fun draftFromPreviewSkipsHintAndInstructionLines() {
        val draft =
            HandOffActions.draftFromPreviewLines(
                listOf(
                    "For: Maya (you choose the thread in Instagram)",
                    "Running ten minutes late",
                    "Operator opens Instagram only. You paste and finish there.",
                ),
            )
        assertEquals("Running ten minutes late", draft)
    }
}
