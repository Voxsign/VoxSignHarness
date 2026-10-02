//
//  SystemAndRoleBar.swift
//  VoiceSign
//
//  打断红色系统条（已生效/未执行/可动作，可关闭）+ 顶部多角色折叠条（Planner/Executor/Verifier）。
//

import SwiftUI

// MARK: - 打断红色系统条

struct SystemBarView: View {
    let bar: SystemBarInfo
    let onRollback: () -> Void
    let onClose: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: "octagon.fill").foregroundColor(.white)
                Text(bar.title).font(.system(size: 13, weight: .bold)).foregroundColor(.white)
                Spacer()
                Button { onClose() } label: {
                    Image(systemName: "xmark").foregroundColor(.white.opacity(0.8))
                }
            }
            ForEach(bar.active, id: \.self) { line in
                Text(line).font(.system(size: 12)).foregroundColor(.white)
            }
            ForEach(bar.blocked, id: \.self) { line in
                Text(line).font(.system(size: 12)).foregroundColor(.white.opacity(0.75))
            }
            HStack(spacing: 8) {
                ForEach(bar.actions, id: \.self) { a in
                    if a == "撤销" {
                        Button("撤销") { onRollback() }
                            .font(.system(size: 12, weight: .bold))
                            .padding(.horizontal, 12).padding(.vertical, 5)
                            .background(Color.white)
                            .foregroundColor(.red)
                            .cornerRadius(8)
                    } else {
                        Button(a) { onClose() }
                            .font(.system(size: 12))
                            .padding(.horizontal, 12).padding(.vertical, 5)
                            .background(Color.white.opacity(0.25))
                            .foregroundColor(.white)
                            .cornerRadius(8)
                    }
                }
            }
        }
        .padding(12)
        .background(Color.red)
        .cornerRadius(14)
    }
}

// MARK: - 多角色折叠条

struct RoleBarView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 6) {
                Circle()
                    .fill(roleColor)
                    .frame(width: 8, height: 8)
                Button {
                    withAnimation { model.roleBarOpen.toggle() }
                } label: {
                    HStack(spacing: 2) {
                        Text(VSLogic.roleLabels[model.activeRole] ?? model.activeRole)
                            .font(.system(size: 13, weight: .semibold))
                        Image(systemName: "chevron.down").font(.system(size: 10))
                    }
                    .foregroundColor(.primary)
                }
                Spacer()
                Text("VoxSign").font(.system(size: 14, weight: .bold))
                Spacer()
                Button {
                    model.showSettings = true
                } label: {
                    Image(systemName: "gearshape.fill").font(.system(size: 16))
                }
            }
            .padding(.horizontal, 14).padding(.vertical, 10)

            if model.roleBarOpen {
                HStack(spacing: 8) {
                    ForEach(model.roles, id: \.id) { r in
                        Text(r.label)
                            .font(.system(size: 12, weight: r.active ? .bold : .regular))
                            .padding(.horizontal, 10).padding(.vertical, 5)
                            .background(r.active ? VSColor.blue.opacity(0.15) : Color.gray.opacity(0.1))
                            .foregroundColor(r.active ? VSColor.blue : .gray)
                            .cornerRadius(8)
                    }
                    Spacer()
                }
                .padding(.horizontal, 14).padding(.bottom, 8)
            }
        }
        .background(VSColor.bg)
    }

    private var roleColor: Color {
        switch model.activeRole {
        case "planner": return .orange
        case "verifier": return .green
        default: return VSColor.blue
        }
    }
}
