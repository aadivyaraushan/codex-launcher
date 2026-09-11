import Foundation
import OperatorCore
import XCTest
@testable import OperatorApp

final class PhoneOAuthClientTests: XCTestCase {
    func testCancelledExchangeDoesNotStoreLateTokens() async throws {
        let entered = expectation(description: "token exchange started")
        let transport = PausedTokenTransport(entered: entered)
        let client = PhoneOAuthClient(provider: .spotify,
            registration: .init(clientID: "client", redirectURI: "app.operator.ios:/oauth"),
            accountID: "test", store: MemoryCredentialStore(), transport: transport)
        let request = try await client.makeAuthorizationRequest()
        let exchange = Task { try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?code=code&state=\(request.state)")!) }
        await fulfillment(of: [entered], timeout: 1)
        exchange.cancel()
        await transport.finish()
        do { _ = try await exchange.value; XCTFail("Cancelled sign-in must not finish") }
        catch { XCTAssertTrue(error is CancellationError) }
        await XCTAssertThrowsErrorAsync(try await client.accessToken()) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .notConnected)
        }
    }
    func testGoogleAuthorizationUsesBothRequiredScopesAndS256PKCE() async throws {
        let client = self.client(provider: .google)

        let request = try await client.makeAuthorizationRequest()
        let query = try XCTUnwrap(URLComponents(url: request.url, resolvingAgainstBaseURL: false)?.queryItems)

        XCTAssertEqual(request.url.host, "accounts.google.com")
        XCTAssertEqual(query.value(for: "response_type"), "code")
        XCTAssertEqual(query.value(for: "code_challenge_method"), "S256")
        XCTAssertEqual(query.value(for: "scope")?.split(separator: " ").map(String.init), OAuthProvider.google.scopes)
        XCTAssertEqual(query.value(for: "redirect_uri"), "app.operator.ios:/oauth")
        XCTAssertEqual(query.value(for: "code_challenge"), request.codeVerifier.s256Challenge)
        XCTAssertGreaterThanOrEqual(request.codeVerifier.count, 43)
        XCTAssertGreaterThanOrEqual(request.state.count, 43)
    }

    func testGoogleCallbackIgnoresDocumentedMetadataAndCompletesExchange() async throws {
        let transport = FixtureTransport(response: .json(
            #"{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"scope":"\#(OAuthProvider.google.scopes.joined(separator: " "))"}"#))
        let client = self.client(provider: .google, transport: transport)
        let request = try await client.makeAuthorizationRequest()
        var callback = URLComponents(string: "app.operator.ios:/oauth")!
        callback.queryItems = [
            .init(name: "code", value: "code"),
            .init(name: "state", value: request.state),
            .init(name: "scope", value: OAuthProvider.google.scopes.joined(separator: " ")),
            .init(name: "authuser", value: "0"),
            .init(name: "prompt", value: "consent"),
            .init(name: "provider_extension", value: "ignored"),
        ]

        let tokens = try await client.completeAuthorizationCallback(try XCTUnwrap(callback.url))

        XCTAssertEqual(tokens.accessToken, "access")
    }

    func testCallbackRejectsDuplicateOAuthKeysBeforeExchange() async throws {
        let transport = FixtureTransport()
        let client = self.client(provider: .google, transport: transport)
        let request = try await client.makeAuthorizationRequest()
        var callback = URLComponents(string: "app.operator.ios:/oauth")!
        callback.queryItems = [
            .init(name: "code", value: "first"),
            .init(name: "code", value: "second"),
            .init(name: "state", value: request.state),
        ]

        await XCTAssertThrowsErrorAsync(try await client.completeAuthorizationCallback(try XCTUnwrap(callback.url))) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .callbackRedirectMismatch)
        }
        let requests = await transport.requests
        XCTAssertTrue(requests.isEmpty)
    }

    func testSlackUsesOnlyUserScopesAndNoBotScope() async throws {
        let request = try await self.client(provider: .slack).makeAuthorizationRequest()
        let query = try XCTUnwrap(URLComponents(url: request.url, resolvingAgainstBaseURL: false)?.queryItems)

        XCTAssertNil(query.value(for: "scope"))
        XCTAssertEqual(query.value(for: "user_scope"), OAuthProvider.slack.scopes.joined(separator: ","))
        XCTAssertEqual(query.value(for: "code_challenge_method"), "S256")
    }

    func testMissingRegistrationFailsBeforeCreatingAuthorizationRequest() async {
        let client = PhoneOAuthClient(
            provider: .spotify,
            registration: .init(clientID: "", redirectURI: ""),
            accountID: "person-a",
            store: MemoryCredentialStore(),
            transport: FixtureTransport())

        await XCTAssertThrowsErrorAsync(try await client.makeAuthorizationRequest()) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .missingRegistration)
        }
    }

    func testCallbackRejectsWrongRedirectAndProviderErrorBeforeExchange() async throws {
        let transport = FixtureTransport()
        let client = self.client(provider: .spotify, transport: transport)
        let request = try await client.makeAuthorizationRequest()

        await XCTAssertThrowsErrorAsync(try await client.completeAuthorizationCallback(URL(string: "app.other:/oauth?code=code&state=\(request.state)")!)) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .callbackRedirectMismatch)
        }
        await XCTAssertThrowsErrorAsync(try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?error=access_denied&state=\(request.state)")!)) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .authorizationDenied)
        }
        let requests = await transport.requests
        XCTAssertTrue(requests.isEmpty)
    }

    func testCallbackRequiresConfiguredRedirectQueryAndAcceptsItsOAuthParameters() async throws {
        let transport = FixtureTransport()
        let client = PhoneOAuthClient(
            provider: .spotify,
            registration: .init(clientID: "client-id", redirectURI: "app.operator.ios:/oauth?route=callback"),
            accountID: "person-a",
            store: MemoryCredentialStore(),
            transport: transport,
            randomBytes: { count in Data((0 ..< count).map { UInt8($0) }) },
            now: { Date(timeIntervalSince1970: 1_000) })
        let request = try await client.makeAuthorizationRequest()

        await XCTAssertThrowsErrorAsync(try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?code=code&state=\(request.state)")!)) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .callbackRedirectMismatch)
        }
        let tokens = try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?route=callback&code=code&state=\(request.state)")!)

        XCTAssertEqual(tokens.accessToken, "access")
    }

    func testCodeExchangePostsPublicClientFormAndStoresSeparatedAccountToken() async throws {
        let transport = FixtureTransport(response: .json(
            #"{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"scope":"\#(OAuthProvider.google.scopes.joined(separator: " "))"}"#))
        let store = MemoryCredentialStore()
        let client = self.client(provider: .google, accountID: "person-a", store: store, transport: transport)
        let request = try await client.makeAuthorizationRequest()

        let tokens = try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?code=code-1&state=\(request.state)")!)

        XCTAssertEqual(tokens.accessToken, "access")
        XCTAssertEqual(tokens.refreshToken, "refresh")
        XCTAssertEqual(Set(tokens.grantedScopes), Set(OAuthProvider.google.scopes))
        let requests = await transport.requests
        let http = try XCTUnwrap(requests.first)
        XCTAssertEqual(http.httpMethod, "POST")
        XCTAssertEqual(http.value(forHTTPHeaderField: "Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertNil(http.value(forHTTPHeaderField: "Authorization"))
        let form = try XCTUnwrap(String(data: http.httpBody ?? Data(), encoding: .utf8)).formValues
        XCTAssertEqual(form["grant_type"], "authorization_code")
        XCTAssertEqual(form["client_id"], "client-id")
        XCTAssertEqual(form["code"], "code-1")
        XCTAssertEqual(form["redirect_uri"], "app.operator.ios:/oauth")
        XCTAssertEqual(form["code_verifier"], request.codeVerifier)
        let storedAccessToken = try await client.accessToken()
        XCTAssertEqual(storedAccessToken, "access")
        let otherProvider = self.client(provider: .spotify, accountID: "person-a", store: store, transport: transport)
        await XCTAssertThrowsErrorAsync(try await otherProvider.accessToken()) { error in
            XCTAssertEqual(error as? PhoneOAuthError, .notConnected)
        }
    }

    func testMicrosoftTokenResponseOnlyRequiresAccessTokenScopes() async throws {
        let transport = FixtureTransport(response: .json(
            #"{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"scope":"\#(OAuthProvider.microsoftOutlook.requiredAccessTokenScopes.joined(separator: " "))"}"#))
        let client = self.client(provider: .microsoftOutlook, transport: transport)
        let request = try await client.makeAuthorizationRequest()

        let tokens = try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?code=code&state=\(request.state)")!)

        XCTAssertEqual(tokens.accessToken, "access")
        XCTAssertEqual(tokens.refreshToken, "refresh")
        // The point of this test is that openid and offline_access are sign-in
        // metadata rather than API permissions, so they are absent from what
        // the token grants. Derived so adding a Graph scope does not break it.
        XCTAssertEqual(Set(tokens.grantedScopes), Set(OAuthProvider.microsoftOutlook.requiredAccessTokenScopes))
        XCTAssertFalse(tokens.grantedScopes.contains("openid"))
        XCTAssertFalse(tokens.grantedScopes.contains("offline_access"))
    }

    func testRefreshKeepsExistingRefreshTokenWhenProviderOmitsReplacement() async throws {
        let transport = FixtureTransport(response: .json(#"{"access_token":"fresh-access","token_type":"Bearer","expires_in":1800,"scope":"user-read-playback-state user-modify-playback-state"}"#))
        let store = MemoryCredentialStore()
        let client = self.client(provider: .spotify, store: store, transport: transport)
        try await client.storeForTesting(.init(accessToken: "old-access", refreshToken: "old-refresh", expiresAt: .distantPast, grantedScopes: OAuthProvider.spotify.scopes))

        let tokens = try await client.refresh()

        XCTAssertEqual(tokens.accessToken, "fresh-access")
        XCTAssertEqual(tokens.refreshToken, "old-refresh")
        let requests = await transport.requests
        let http = try XCTUnwrap(requests.first)
        let form = try XCTUnwrap(String(data: http.httpBody ?? Data(), encoding: .utf8)).formValues
        XCTAssertEqual(form["grant_type"], "refresh_token")
        XCTAssertEqual(form["refresh_token"], "old-refresh")
        XCTAssertEqual(form["client_id"], "client-id")
    }

    func testSlackUserTokenExchangeUsesPKCEFormWithoutSecretOrBotToken() async throws {
        let transport = FixtureTransport(response: .json(#"{"ok":true,"authed_user":{"access_token":"user-access","token_type":"user","refresh_token":"refresh","expires_in":3600,"scope":"chat:write,channels:read,channels:history,groups:read,groups:history,im:write,im:history,users:read"}}"#))
        let client = self.client(provider: .slack, transport: transport)
        let request = try await client.makeAuthorizationRequest()

        _ = try await client.completeAuthorizationCallback(URL(string: "app.operator.ios:/oauth?code=code%2B1&state=\(request.state)")!)

        let requests = await transport.requests
        let http = try XCTUnwrap(requests.first)
        XCTAssertEqual(http.url, OAuthProvider.slack.tokenEndpoint)
        XCTAssertNil(http.value(forHTTPHeaderField: "Authorization"))
        XCTAssertTrue(String(decoding: http.httpBody ?? Data(), as: UTF8.self).contains("code=code%2B1"))
        let form = try XCTUnwrap(String(data: http.httpBody ?? Data(), encoding: .utf8)).formValues
        XCTAssertEqual(form["code_verifier"], request.codeVerifier)
        XCTAssertNil(form["client_secret"])
    }

    func testSlackRefreshAcceptsOnlyUserTokensAtResponseRoot() async throws {
        for type in ["user", "bot"] {
            let body = #"{"ok":true,"access_token":"fresh","token_type":"TOKEN_TYPE","refresh_token":"rotated","expires_in":3600,"scope":"chat:write,channels:read,channels:history,groups:read,groups:history,im:write,im:history,users:read"}"#.replacingOccurrences(of: "TOKEN_TYPE", with: type)
            let client = self.client(provider: .slack, transport: FixtureTransport(response: .json(body)))
            try await client.storeForTesting(.init(accessToken: "old", refreshToken: "refresh", expiresAt: .distantPast, grantedScopes: OAuthProvider.slack.scopes))
            do {
                let result = try await client.refresh()
                XCTAssertEqual(type, "user")
                XCTAssertEqual(result.refreshToken, "rotated")
            } catch {
                XCTAssertEqual(type, "bot")
                XCTAssertEqual(error as? PhoneOAuthError, .invalidTokenResponse)
            }
        }
    }

    func testExpiredTokenConcurrentReadsShareOneRefreshRequest() async throws {
        let transport = FixtureTransport()
        let client = self.client(provider: .spotify, transport: transport)
        try await client.storeForTesting(.init(accessToken: "old", refreshToken: "refresh", expiresAt: .distantPast, grantedScopes: OAuthProvider.spotify.scopes))

        async let first: String = client.accessToken()
        async let second: String = client.accessToken()
        let tokens = try await [first, second]

        XCTAssertEqual(tokens, ["access", "access"])
        let requests = await transport.requests
        XCTAssertEqual(requests.count, 1)
    }

    private func client(
        provider: OAuthProvider,
        accountID: String = "person-a",
        store: MemoryCredentialStore = MemoryCredentialStore(),
        transport: FixtureTransport = FixtureTransport()) -> PhoneOAuthClient
    {
        PhoneOAuthClient(
            provider: provider,
            registration: .init(clientID: "client-id", redirectURI: "app.operator.ios:/oauth"),
            accountID: accountID,
            store: store,
            transport: transport,
            randomBytes: { count in Data((0 ..< count).map { UInt8($0) }) },
            now: { Date(timeIntervalSince1970: 1_000) })
    }
}

private actor MemoryCredentialStore: CredentialDataStore {
    var value: Data?
    func load() async throws -> Data? { self.value }
    func save(_ data: Data) async throws { self.value = data }
}

private actor PausedTokenTransport: PhoneHTTPTransport {
    let entered: XCTestExpectation
    private var continuation: CheckedContinuation<Void, Never>?
    init(entered: XCTestExpectation) { self.entered = entered }
    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        await withCheckedContinuation { continuation in
            self.continuation = continuation
            entered.fulfill()
        }
        let body = #"{"access_token":"late-token","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"scope":"user-read-playback-state user-modify-playback-state"}"#
        return (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!)
    }
    func finish() { continuation?.resume(); continuation = nil }
}

private actor FixtureTransport: PhoneHTTPTransport {
    enum Response { case json(String) }
    private let response: Response
    private(set) var requests: [URLRequest] = []

    init(response: Response = .json(#"{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"scope":"user-read-playback-state user-modify-playback-state"}"#)) {
        self.response = response
    }

    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        self.requests.append(request)
        let body: String
        switch self.response { case let .json(json): body = json }
        guard let url = request.url, let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: nil) else {
            throw URLError(.badURL)
        }
        return (Data(body.utf8), response)
    }
}

private extension Array where Element == URLQueryItem {
    func value(for name: String) -> String? { self.first(where: { $0.name == name })?.value }
}

private extension String {
    var formValues: [String: String] {
        URLComponents(string: "https://operator.invalid/?\(self)")?.queryItems?.reduce(into: [:]) { $0[$1.name] = $1.value } ?? [:]
    }
}

private func XCTAssertThrowsErrorAsync<T>(
    _ expression: @autoclosure () async throws -> T,
    _ handler: @escaping (Error) -> Void) async
{
    do { _ = try await expression(); XCTFail("Expected error") }
    catch { handler(error) }
}
