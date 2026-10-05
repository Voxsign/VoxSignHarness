//
//  AttachmentPanelView.swift
//  VoxSign
//
//  「＋ 添加资料」Sheet 面板（聊天面视图 T3 改造，新建）：
//  - 顶部：当前待发送附件列表（model.pendingAttachments），每条 kind 图标 + title，可删除
//  - 四个豆包式清单添加入口：文本粘贴 / URL / 图片（PhotosPicker）/ 文件（fileImporter）
//  - 添加成功即 model.addAttachment；发送由 model.send() 自动携带并清空
//  - 未来扩展点：转发邮件（Mail 附件）入口预留，见下方 MARK
//
//  依赖冻结接口（数据层子代理实现，本视图只调用）：
//    Attachment / AttachmentKind；AppModel.pendingAttachments / addAttachment(_:) / removeAttachment(_:)
//

import SwiftUI
import PhotosUI
import UniformTypeIdentifiers

struct AttachmentPanelView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss

    // 表单输入
    @State private var pasteText: String = ""
    @State private var urlString: String = ""
    @State private var pickedItem: PhotosPickerItem? = nil

    var body: some View {
        NavigationStack {
            List {
                // MARK: 待发送附件
                if !model.pendingAttachments.isEmpty {
                    Section("待发送资料（\(model.pendingAttachments.count)）") {
                        ForEach(model.pendingAttachments) { att in
                            AttachmentRow(att: att, onDelete: { model.removeAttachment(att.id) })
                                .swipeActions {
                                    Button(role: .destructive) {
                                        model.removeAttachment(att.id)
                                    } label: {
                                        Label("删除", systemImage: "trash")
                                    }
                                }
                        }
                    }
                }

                // MARK: 文本粘贴
                Section("粘贴文本") {
                    TextEditor(text: $pasteText)
                        .frame(minHeight: 70)
                    HStack {
                        Button {
                            pasteText = UIPasteboard.general.string ?? ""
                        } label: {
                            Label("从剪贴板读取", systemImage: "doc.on.clipboard")
                                .font(.system(size: 13))
                        }
                        Spacer()
                        Button("添加") { addText() }
                            .disabled(pasteText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
                }

                // MARK: URL
                Section("链接") {
                    TextField("https://…", text: $urlString)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    HStack {
                        Spacer()
                        Button("添加") { addURL() }
                            .disabled(URL(string: urlString.trimmingCharacters(in: .whitespaces)) == nil)
                    }
                }

                // MARK: 图片
                Section("图片") {
                    PhotosPicker(selection: $pickedItem, matching: .images) {
                        Label("从相册选择图片", systemImage: "photo")
                            .font(.system(size: 15))
                    }
                    .onChange(of: pickedItem) { newItem in
                        Task { await loadImage(newItem) }
                    }
                }

                // MARK: 文件
                Section {
                    Button {
                        showFilePicker = true
                    } label: {
                        Label("选择文件（PDF / 文本 / 图片…）", systemImage: "doc")
                            .font(.system(size: 15))
                    }
                } footer: {
                    Text("选择后将作为文件附件随消息发送。")
                        .font(.system(size: 12))
                }

                // 未来扩展点：转发邮件（Mail 附件 / 邮件正文）入口——
                //   新增 Section("邮件") + 调用 Mail 组合，Attachment(kind: .text/.file, …)。
                //   届时在此插入：Button("转发邮件") { /* open Mail composer */ }。
            }
            .navigationTitle("添加资料")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("完成") { dismiss() }
                }
            }
        }
        .fileImporter(
            isPresented: $showFilePicker,
            allowedContentTypes: [.data, .pdf, .text, .image],
            allowsMultipleSelection: false
        ) { result in
            handleFileImport(result)
        }
    }

    // MARK: 文件选择器触发状态
    @State private var showFilePicker: Bool = false

    // MARK: - 添加动作

    private func addText() {
        let content = pasteText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !content.isEmpty else { return }
        model.addAttachment(Attachment(id: UUID().uuidString,
                                       kind: .text,
                                       title: "文本",
                                       text: content,
                                       fileName: nil,
                                       localPath: nil))
        pasteText = ""
        dismiss()
    }

    private func addURL() {
        let s = urlString.trimmingCharacters(in: .whitespaces)
        guard URL(string: s) != nil else { return }
        model.addAttachment(Attachment(id: UUID().uuidString,
                                       kind: .url,
                                       title: "链接",
                                       text: s,
                                       fileName: nil,
                                       localPath: nil))
        urlString = ""
        dismiss()
    }

    @MainActor
    private func loadImage(_ item: PhotosPickerItem?) async {
        guard let item else { return }
        guard let data = try? await item.loadTransferable(type: Data.self),
              let uiImage = UIImage(data: data) else { return }
        // 存到应用文档目录，得到稳定 localPath。
        let fm = FileManager.default
        guard let docs = try? fm.url(for: .documentDirectory, in: .userDomainMask, appropriateFor: nil, create: true) else { return }
        let name = "att_img_\(UUID().uuidString).png"
        let url = docs.appendingPathComponent(name)
        guard let png = uiImage.pngData() else { return }
        do {
            try png.write(to: url)
        } catch { return }
        model.addAttachment(Attachment(id: UUID().uuidString,
                                       kind: .image,
                                       title: "图片",
                                       text: nil,
                                       fileName: name,
                                       localPath: url.path))
        pickedItem = nil
        dismiss()
    }

    private func handleFileImport(_ result: Result<[URL], Error>) {
        guard case .success(let urls) = result, let src = urls.first else { return }
        // 安全作用域访问 + 拷贝到文档目录，得到稳定 localPath。
        let scoped = src.startAccessingSecurityScopedResource()
        defer { if scoped { src.stopAccessingSecurityScopedResource() } }
        let fm = FileManager.default
        guard let docs = try? fm.url(for: .documentDirectory, in: .userDomainMask, appropriateFor: nil, create: true) else { return }
        let name = src.lastPathComponent
        let dst = docs.appendingPathComponent("att_" + UUID().uuidString + "_" + name)
        do {
            if fm.fileExists(atPath: dst.path) { try fm.removeItem(at: dst) }
            try fm.copyItem(at: src, to: dst)
        } catch { return }
        model.addAttachment(Attachment(id: UUID().uuidString,
                                       kind: .file,
                                       title: name,
                                       text: nil,
                                       fileName: name,
                                       localPath: dst.path))
        dismiss()
    }
}

// MARK: - 单条附件行（kind 图标 + title）

private struct AttachmentRow: View {
    let att: Attachment
    let onDelete: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: iconName(att.kind))
                .font(.system(size: 16))
                .foregroundColor(VSColor.blue)
                .frame(width: 24)
            Text(att.title)
                .font(.system(size: 15))
                .lineLimit(1)
            Spacer()
            Button(action: onDelete) {
                Image(systemName: "xmark.circle.fill")
                    .font(.system(size: 15))
                    .foregroundColor(.secondary)
            }
            .buttonStyle(.plain)
        }
    }

    private func iconName(_ kind: AttachmentKind) -> String {
        switch kind {
        case .text: return "doc.text"
        case .url:  return "link"
        case .image: return "photo"
        case .file: return "doc"
        }
    }
}
