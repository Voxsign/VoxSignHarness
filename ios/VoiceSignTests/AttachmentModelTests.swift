//
//  AttachmentModelTests.swift
//  VoiceSignTests
//
//  v2.4 附件模型单测：四种 kind 构造、Codable roundtrip、附加到 Bubble。
//

import XCTest
@testable import VoiceSign

final class AttachmentModelTests: XCTestCase {

    func testAllKindsConstruct() {
        let text = Attachment(id: "1", kind: .text, title: "粘贴文本", text: "正文内容")
        let url = Attachment(id: "2", kind: .url, title: "参考链接", text: "https://voxsign.ai")
        let image = Attachment(id: "3", kind: .image, title: "截图", localPath: "/tmp/a.png")
        let file = Attachment(id: "4", kind: .file, title: "报告", fileName: "report.pdf", localPath: "/tmp/report.pdf")

        XCTAssertEqual(text.kind, .text)
        XCTAssertEqual(url.kind, .url)
        XCTAssertEqual(image.kind, .image)
        XCTAssertEqual(file.kind, .file)
        XCTAssertEqual(file.fileName, "report.pdf")
    }

    func testKindRawValues() {
        XCTAssertEqual(AttachmentKind.text.rawValue, "text")
        XCTAssertEqual(AttachmentKind.url.rawValue, "url")
        XCTAssertEqual(AttachmentKind.image.rawValue, "image")
        XCTAssertEqual(AttachmentKind.file.rawValue, "file")
    }

    func testCodableRoundtrip() throws {
        let original = Attachment(id: "abc", kind: .file, title: "合同",
                                  text: "可选备注", fileName: "contract.docx",
                                  localPath: "/docs/contract.docx")
        let data = try JSONEncoder().encode(original)
        let decoded = try JSONDecoder().decode(Attachment.self, from: data)
        XCTAssertEqual(original, decoded)
        XCTAssertEqual(decoded.kind, .file)
        XCTAssertEqual(decoded.text, "可选备注")
        XCTAssertEqual(decoded.fileName, "contract.docx")
    }

    func testCodableRoundtripOptionalNils() throws {
        // 仅 kind + title，其余 nil：编解码不得崩溃且保持等价。
        let original = Attachment(id: "x", kind: .url, title: "链接")
        let data = try JSONEncoder().encode(original)
        let decoded = try JSONDecoder().decode(Attachment.self, from: data)
        XCTAssertEqual(original, decoded)
        XCTAssertNil(decoded.text)
        XCTAssertNil(decoded.fileName)
        XCTAssertNil(decoded.localPath)
    }

    func testAttachToBubble() {
        let att = Attachment(id: "a1", kind: .text, title: "备注", text: "随消息提交的资料")
        let bubble = Bubble(text: "帮我整理下", fromVoice: false, attachments: [att])
        XCTAssertEqual(bubble.attachments.count, 1)
        XCTAssertEqual(bubble.attachments.first?.kind, .text)
        XCTAssertEqual(bubble.attachments.first?.title, "备注")
    }

    func testStoredMessageCarriesAttachments() throws {
        let att = Attachment(id: "a2", kind: .image, title: "图", localPath: "/tmp/i.jpg")
        let msg = StoredMessage(id: "m1", role: "user", text: "看图", attachments: [att])
        let data = try JSONEncoder().encode(msg)
        let decoded = try JSONDecoder().decode(StoredMessage.self, from: data)
        XCTAssertEqual(decoded.attachments.count, 1)
        XCTAssertEqual(decoded.attachments.first?.kind, .image)
    }
}
