//
//  TouchCaptureView.swift
//  VoxSign
//
//  【按住延迟修复 T5】用 UIKit 触摸回调（touchesBegan/Ended/Cancelled/Moved）替换
//  SwiftUI DragGesture：touchesBegan 在 touch down 同步回调（豆包原生同款零延迟），
//  而 SwiftUI 手势识别需要系统事件分发（首帧明显滞后几十毫秒甚至上百毫秒）→
//  按下→震动 + 视觉按住态在手指落下的同一帧出现。
//  上滑取消语义保持：dy = 当前y - 按下y（负=上移），阈值 -80pt，可滑回（豆包同款）。
//
import SwiftUI
import UIKit

struct TouchCaptureView<Content: View>: UIViewRepresentable {
    var onTouchDown: () -> Void
    var onTouchMove: (CGFloat) -> Void
    var onTouchUp: () -> Void
    var onTouchCancel: () -> Void
    @ViewBuilder var content: () -> Content

    func makeUIView(context: Context) -> TouchCaptureUIView {
        let v = TouchCaptureUIView()
        v.onTouchDown = onTouchDown
        v.onTouchMove = onTouchMove
        v.onTouchUp = onTouchUp
        v.onTouchCancel = onTouchCancel
        let host = UIHostingController(rootView: content())
        host.view.backgroundColor = .clear
        // 内容仅展示不交互：触摸全部由 TouchCaptureUIView 接收（本容器只做手势采集）。
        host.view.isUserInteractionEnabled = false
        host.view.frame = v.bounds
        host.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        v.addSubview(host.view)
        return v
    }

    func updateUIView(_ uiView: TouchCaptureUIView, context: Context) {
        uiView.onTouchDown = onTouchDown
        uiView.onTouchMove = onTouchMove
        uiView.onTouchUp = onTouchUp
        uiView.onTouchCancel = onTouchCancel
    }
}

/// 触摸容器：touchesBegan = touch down 同步回调（比 SwiftUI 手势早一拍）；
/// 上滑位移 = 按下点至当前点 y 差值（负 = 上移）。
final class TouchCaptureUIView: UIView {
    var onTouchDown: () -> Void = {}
    var onTouchMove: (CGFloat) -> Void = { _ in }
    var onTouchUp: () -> Void = {}
    var onTouchCancel: () -> Void = {}

    private var startY: CGFloat = 0

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        startY = touches.first?.location(in: self).y ?? 0
        onTouchDown()
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let t = touches.first else { return }
        onTouchMove(t.location(in: self).y - startY)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        onTouchUp()
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        onTouchCancel()
    }
}
