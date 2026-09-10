import SwiftUI
import WidgetKit

struct OperatorWidgetEntry: TimelineEntry {
    let date: Date
}

struct OperatorWidgetProvider: TimelineProvider {
    func placeholder(in context: Context) -> OperatorWidgetEntry {
        OperatorWidgetEntry(date: .now)
    }

    func getSnapshot(in context: Context, completion: @escaping (OperatorWidgetEntry) -> Void) {
        completion(OperatorWidgetEntry(date: .now))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<OperatorWidgetEntry>) -> Void) {
        completion(Timeline(entries: [OperatorWidgetEntry(date: .now)], policy: .never))
    }
}

struct OperatorWidgetView: View {
    @Environment(\.widgetFamily) private var family

    var body: some View {
        switch self.family {
        case .accessoryCircular:
            Image(systemName: "message")
                .accessibilityLabel("Open Operator")
        case .accessoryRectangular:
            Label("Open Operator", systemImage: "message")
        default:
            VStack(spacing: 8) {
                Image(systemName: "message")
                    .font(.title2)
                Text("Open Operator")
                    .font(.headline)
            }
            .containerBackground(for: .widget) {
                Color(uiColor: .systemBackground)
            }
        }
    }
}

struct OperatorWidget: Widget {
    let kind = "OperatorWidget"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: self.kind, provider: OperatorWidgetProvider()) { _ in
            OperatorWidgetView()
        }
        .configurationDisplayName("Operator")
        .description("Open your Operator chat.")
        .supportedFamilies([.systemSmall, .accessoryCircular, .accessoryRectangular])
    }
}

@main
struct OperatorWidgetBundle: WidgetBundle {
    var body: some Widget {
        OperatorWidget()
    }
}
