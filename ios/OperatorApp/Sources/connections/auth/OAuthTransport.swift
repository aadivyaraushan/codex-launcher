import Foundation

protocol PhoneHTTPTransport: Sendable {
    func data(for request: URLRequest) async throws -> (Data, URLResponse)
}

struct URLSessionPhoneHTTPTransport: PhoneHTTPTransport {
    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        let delegate = RejectRedirectsDelegate()
        let result = try await URLSession.shared.data(for: request, delegate: delegate)
        if let response = result.1 as? HTTPURLResponse, (300 ... 399).contains(response.statusCode) {
            throw URLError(.httpTooManyRedirects)
        }
        return result
    }
}

private final class RejectRedirectsDelegate: NSObject, URLSessionTaskDelegate {
    func urlSession(
        _: URLSession,
        task _: URLSessionTask,
        willPerformHTTPRedirection _: HTTPURLResponse,
        newRequest _: URLRequest,
        completionHandler: @escaping (URLRequest?) -> Void)
    {
        completionHandler(nil)
    }
}
