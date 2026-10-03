import Foundation
import Libbox

public class CommandClient: ObservableObject {
    public enum ConnectionType {
        case status
        case groups
        case log
        case clashMode
        case connections
    }

    private let connectionType: ConnectionType
    private let logMaxLines: Int
    private var commandClient: LibboxCommandClient?
    private var connectTask: Task<Void, Error>?
    @Published public var isConnected: Bool
    @Published public var status: LibboxStatusMessage?
    @Published public var groups: [LibboxOutboundGroup]?
    @Published public var logList: [String]
    @Published public var clashModeList: [String]
    @Published public var clashMode: String

    @Published public var connectionStateFilter = ConnectionStateFilter.active
    @Published public var connectionSort = ConnectionSort.byDate
    @Published public var connections: [LibboxConnection]?
    public var rawConnections: LibboxConnections?

    public init(_ connectionType: ConnectionType, logMaxLines: Int = 300) {
        self.connectionType = connectionType
        self.logMaxLines = logMaxLines
        logList = []
        clashModeList = []
        clashMode = ""
        isConnected = false
    }

    public func connect() {
        if isConnected {
            return
        }
        if let connectTask {
            connectTask.cancel()
        }
        connectTask = Task {
            await connect0()
        }
    }

    public func disconnect() {
        if let connectTask {
            connectTask.cancel()
            self.connectTask = nil
        }
        if let commandClient {
            try? commandClient.disconnect()
            self.commandClient = nil
        }
    }

    public func filterConnectionsNow() {
        guard let message = rawConnections else {
            return
        }
        connections = filterConnections(message)
    }

    private func filterConnections(_ message: LibboxConnections) -> [LibboxConnection] {
        message.filterState(Int32(connectionStateFilter.rawValue))
        switch connectionSort {
        case .byDate:
            message.sortByDate()
        case .byTraffic:
            message.sortByTraffic()
        case .byTrafficTotal:
            message.sortByTrafficTotal()
        }
        let connectionIterator = message.iterator()!
        var connections: [LibboxConnection] = []
        while connectionIterator.hasNext() {
            connections.append(connectionIterator.next()!)
        }
        return connections
    }

    private func initializeConnectionFilterState() async {
        let newFilter: ConnectionStateFilter = await .init(rawValue: SharedPreferences.connectionStateFilter.get()) ?? .active
        let newSort: ConnectionSort = await .init(rawValue: SharedPreferences.connectionSort.get()) ?? .byDate
        await MainActor.run {
            connectionStateFilter = newFilter
            connectionSort = newSort
        }
    }

    private nonisolated func connect0() async {
        if connectionType == .connections {
            await initializeConnectionFilterState()
        }

        let clientOptions = LibboxCommandClientOptions()
        switch connectionType {
        case .status:
            clientOptions.addCommand(LibboxCommandStatus)
        case .groups:
            clientOptions.addCommand(LibboxCommandGroup)
        case .log:
            clientOptions.addCommand(LibboxCommandLog)
        case .clashMode:
            clientOptions.addCommand(LibboxCommandClashMode)
        case .connections:
            clientOptions.addCommand(LibboxCommandConnections)
        }
        switch connectionType {
        case .log:
            clientOptions.statusInterval = Int64(500 * NSEC_PER_MSEC)
        default:
            clientOptions.statusInterval = Int64(NSEC_PER_SEC)
        }
        let client = LibboxNewCommandClient(clientHandler(self), clientOptions)!
        do {
            for i in 0 ..< 10 {
                try await Task.sleep(nanoseconds: UInt64(Double(100 + (i * 50)) * Double(NSEC_PER_MSEC)))
                try Task.checkCancellation()
                do {
                    try client.connect()
                    await MainActor.run {
                        commandClient = client
                    }
                    return
                } catch {}
                try Task.checkCancellation()
            }
        } catch {
            try? client.disconnect()
        }
    }

    private class clientHandler: NSObject, LibboxCommandClientHandlerProtocol {
        private let commandClient: CommandClient

        init(_ commandClient: CommandClient) {
            self.commandClient = commandClient
        }

        func connected() {
            DispatchQueue.main.async { [self] in
                if commandClient.connectionType == .log {
                    commandClient.logList = []
                }
                commandClient.isConnected = true
            }
        }

        func disconnected(_ message: String?) {
            DispatchQueue.main.async { [self] in
                commandClient.isConnected = false
            }
            if let message {
                NSLog("client disconnected: \(message)")
            }
        }

        func clearLogs() {
            DispatchQueue.main.async { [self] in
                commandClient.logList.removeAll()
            }
        }

        func writeLogs(_ messageList: LibboxLogIteratorProtocol?) {
            guard let messageList else {
                return
            }
            DispatchQueue.main.async { [self] in
                var newLogList = commandClient.logList
                while messageList.hasNext() {
                    let entry = messageList.next()!
                    newLogList.append(entry.message)
                }
                if newLogList.count >= commandClient.logMaxLines {
                    newLogList.removeSubrange(0 ... newLogList.count - commandClient.logMaxLines)
                }
                commandClient.logList = newLogList
            }
        }

        func setDefaultLogLevel(_: Int32) {}

        func write(_ events: LibboxConnectionEvents?) {}

        func writeOutbounds(_: (any LibboxOutboundGroupItemIteratorProtocol)?) {}

        func writeStatus(_ message: LibboxStatusMessage?) {
            DispatchQueue.main.async { [self] in
                commandClient.status = message
            }
        }

        func writeGroups(_ groups: LibboxOutboundGroupIteratorProtocol?) {
            guard let groups else {
                return
            }
            var newGroups: [LibboxOutboundGroup] = []
            while groups.hasNext() {
                newGroups.append(groups.next()!)
            }
            DispatchQueue.main.async { [self] in
                commandClient.groups = newGroups
            }
        }

        func initializeClashMode(_ modeList: LibboxStringIteratorProtocol?, currentMode: String?) {
            DispatchQueue.main.async { [self] in
                commandClient.clashModeList = modeList!.toArray()
                commandClient.clashMode = currentMode!
            }
        }

        func updateClashMode(_ newMode: String?) {
            DispatchQueue.main.async { [self] in
                commandClient.clashMode = newMode!
            }
        }
    }
}

public enum ConnectionStateFilter: Int, CaseIterable, Identifiable {
    public var id: Self {
        self
    }

    case all
    case active
    case closed
}

public extension ConnectionStateFilter {
    var name: String {
        switch self {
        case .all:
            return NSLocalizedString("All", comment: "")
        case .active:
            return NSLocalizedString("Active", comment: "")
        case .closed:
            return NSLocalizedString("Closed", comment: "")
        }
    }
}

public enum ConnectionSort: Int, CaseIterable, Identifiable {
    public var id: Self {
        self
    }

    case byDate
    case byTraffic
    case byTrafficTotal
}

public extension ConnectionSort {
    var name: String {
        switch self {
        case .byDate:
            return NSLocalizedString("Date", comment: "")
        case .byTraffic:
            return NSLocalizedString("Traffic", comment: "")
        case .byTrafficTotal:
            return NSLocalizedString("Traffic Total", comment: "")
        }
    }
}
