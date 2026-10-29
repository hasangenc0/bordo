import SwiftUI
import UIKit

/// A UITextView-backed multi-line field. SwiftUI's `TextField(axis: .vertical)` re-lays-out on every
/// keystroke and is noticeably laggy in long conversations; UIKit's text view is not.
struct GrowingTextView: UIViewRepresentable {
    @Binding var text: String
    var placeholder: String
    var maxLines = 6
    var identifier = "composer"

    func makeUIView(context: Context) -> UITextView {
        let v = UITextView()
        v.delegate = context.coordinator
        v.font = .preferredFont(forTextStyle: .body)
        v.adjustsFontForContentSizeCategory = true
        v.backgroundColor = .clear
        v.textContainerInset = UIEdgeInsets(top: 11, left: 0, bottom: 11, right: 0)
        v.textContainer.lineFragmentPadding = 0
        v.isScrollEnabled = false
        v.autocorrectionType = .default
        v.accessibilityIdentifier = identifier
        v.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let label = UILabel()
        label.text = placeholder
        label.font = v.font
        label.adjustsFontForContentSizeCategory = true
        label.textColor = .placeholderText
        label.translatesAutoresizingMaskIntoConstraints = false
        v.addSubview(label)
        NSLayoutConstraint.activate([
            label.leadingAnchor.constraint(equalTo: v.leadingAnchor),
            label.topAnchor.constraint(equalTo: v.topAnchor, constant: 11),
        ])
        context.coordinator.placeholder = label
        return v
    }

    func updateUIView(_ v: UITextView, context: Context) {
        // Only push SwiftUI -> UIKit when it actually differs (e.g. cleared after send);
        // writing back every update would fight the user's typing.
        if v.text != text { v.text = text }
        context.coordinator.placeholder?.isHidden = !text.isEmpty
        context.coordinator.parent = self
    }

    func sizeThatFits(_ proposal: ProposedViewSize, uiView v: UITextView, context: Context) -> CGSize? {
        let width = proposal.width ?? 300
        let line = v.font?.lineHeight ?? 20
        let maxHeight = line * CGFloat(maxLines) + v.textContainerInset.top + v.textContainerInset.bottom
        let fitting = v.sizeThatFits(CGSize(width: width, height: .greatestFiniteMagnitude)).height
        let h = min(max(fitting, line + v.textContainerInset.top + v.textContainerInset.bottom), maxHeight)
        let scrolls = fitting > maxHeight
        if v.isScrollEnabled != scrolls { DispatchQueue.main.async { v.isScrollEnabled = scrolls } }
        return CGSize(width: width, height: h)
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    final class Coordinator: NSObject, UITextViewDelegate {
        var parent: GrowingTextView
        weak var placeholder: UILabel?
        init(_ p: GrowingTextView) { parent = p }

        func textViewDidChange(_ tv: UITextView) {
            placeholder?.isHidden = !tv.text.isEmpty
            // Avoid publishing mid-composition (IME marked text) so CJK input isn't disturbed.
            if tv.markedTextRange == nil { parent.text = tv.text }
        }
    }
}
