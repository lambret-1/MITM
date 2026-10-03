import Foundation
import Libbox
import NetworkExtension
#if os(iOS)
    import WidgetKit
#endif
#if os(macOS)
    import CoreLocation
#endif

open class ExtensionProvider: NEPacketTunnelProvider {
    private var commandServer: LibboxCommandServer!
    private var systemProxyAvailable = false
    private var systemProxyEnabled = false
    private var platformInterface: ExtensionPlatformInterface!

    override open func startTunnel(options _: [String: NSObject]?) async throws {
        let options = LibboxSetupOptions()
        options.basePath = FilePath.sharedDirectory.relativePath
        options.workingPath = FilePath.workingDirectory.relativePath
        options.tempPath = FilePath.cacheDirectory.relativePath
        var error: NSError?
        LibboxSetup(options, &error)
        if let error {
            writeFatalError("(packet-tunnel) error: setup service: \(error.localizedDescription)")
            return
        }

        if platformInterface == nil {
            platformInterface = ExtensionPlatformInterface(self)
        }
        commandServer = LibboxNewCommandServer(self, platformInterface, &error)
        if let error {
            writeFatalError("(packet-tunnel) error: create command server: \(error.localizedDescription)")
            return
        }
        do {
            try commandServer.start()
        } catch {
            writeFatalError("(packet-tunnel): log server start error: \(error.localizedDescription)")
            return
        }
        writeMessage("(packet-tunnel): Here I stand")
        await startService()
        #if os(iOS)
            if #available(iOS 18.0, *) {
                ControlCenter.shared.reloadControls(ofKind: ExtensionProfile.controlKind)
            }
        #endif
    }

    func writeMessage(_ message: String) {
        if let commandServer {
            commandServer.writeMessage(0, message: message)
        }
    }

    public func writeFatalError(_ message: String) {
        #if DEBUG
            NSLog(message)
        #endif
        writeMessage(message)
        commandServer?.setError(message)
        cancelTunnelWithError(nil)
    }

    private func startService() async {
        let profile: Profile?
        do {
            profile = try await ProfileManager.get(Int64(SharedPreferences.selectedProfileID.get()))
        } catch {
            writeFatalError("(packet-tunnel) error: read selected profile: \(error.localizedDescription)")
            return
        }
        guard let profile else {
            writeFatalError("(packet-tunnel) error: missing selected profile")
            return
        }
        let configContent: String
        do {
            configContent = try profile.read()
        } catch {
            writeFatalError("(packet-tunnel) error: read config file \(profile.path): \(error.localizedDescription)")
            return
        }
        do {
            try commandServer.startOrReloadService(configContent, options: nil)
        } catch {
            writeFatalError("(packet-tunnel) error: start service: \(error.localizedDescription)")
            return
        }
        #if os(macOS)
            await SharedPreferences.startedByUser.set(true)
            if commandServer.needWIFIState() {
                if !Variant.useSystemExtension {
                    locationManager = CLLocationManager()
                    locationDelegate = stubLocationDelegate(commandServer)
                    locationManager?.delegate = locationDelegate
                    locationManager?.requestLocation()
                } else {
                    commandServer.writeMessage(0, message: "(packet-tunnel) WIFI SSID and BSSID information is not currently available in the standalone version of SFM. We are working on resolving this issue.")
                }
            }
        #endif
    }

    #if os(macOS)

        private var locationManager: CLLocationManager?
        private var locationDelegate: stubLocationDelegate?

        class stubLocationDelegate: NSObject, CLLocationManagerDelegate {
            private unowned let commandServer: LibboxCommandServer
            init(_ commandServer: LibboxCommandServer) {
                self.commandServer = commandServer
            }

            func locationManagerDidChangeAuthorization(_: CLLocationManager) {
                commandServer.updateWIFIState()
            }

            func locationManager(_: CLLocationManager, didUpdateLocations _: [CLLocation]) {}

            func locationManager(_: CLLocationManager, didFailWithError _: Error) {}
        }

    #endif

    private func stopService() {
        do {
            try commandServer.closeService()
        } catch {
            writeMessage("(packet-tunnel) error: stop service: \(error.localizedDescription)")
        }
        if let platformInterface {
            platformInterface.reset()
        }
    }

    func reloadService() async {
        writeMessage("(packet-tunnel) reloading service")
        reasserting = true
        defer {
            reasserting = false
        }
        stopService()
        await startService()
    }

    func postServiceClose() {}

    override open func stopTunnel(with reason: NEProviderStopReason) async {
        writeMessage("(packet-tunnel) stopping, reason: \(reason.rawValue)")
        stopService()
        if let server = commandServer {
            try? await Task.sleep(nanoseconds: 100 * NSEC_PER_MSEC)
            try? server.close()
            commandServer = nil
        }
        #if os(macOS)
            if reason == .userInitiated {
                await SharedPreferences.startedByUser.set(reason == .userInitiated)
            }
        #endif
        #if os(iOS)
            if #available(iOS 18.0, *) {
                ControlCenter.shared.reloadControls(ofKind: ExtensionProfile.controlKind)
            }
        #endif
    }

    override open func handleAppMessage(_ messageData: Data) async -> Data? {
        messageData
    }

    override open func sleep() async {
        commandServer?.pause()
    }

    override open func wake() {
        commandServer?.wake()
    }
}

extension ExtensionProvider: LibboxCommandServerHandlerProtocol {
    public func connectSSHAgent() throws -> Int32 {
        0
    }

    public func getSystemProxyStatus() throws -> LibboxSystemProxyStatus! {
        platformInterface!.getSystemProxyStatus()
    }

    public func serviceReload() throws {
        Task {
            await self.reloadService()
        }
    }

    public func serviceStop() throws {
        Task {
            self.cancelTunnelWithError(nil)
        }
    }

    public func setSystemProxyEnabled(_: Bool) throws {}

    public func triggerNativeCrash() throws {
        fatalError("native crash")
    }

    public func writeDebugMessage(_ message: String?) {
        writeMessage(message ?? "")
    }
}
