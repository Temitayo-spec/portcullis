import ExpoModulesCore
import StoreKit

/// The iOS half of expo-portcullis: which environment installed this copy
/// (spec/gate-v1.md R8), and the App Store page for the update screen.
///
/// It reports raw facts only. Turning them into a store key happens in
/// JavaScript (storeKeyFor), where the shared spec cases test it.
public class PortcullisModule: Module {
  /// Kept here while the App Store sheet is up: the sheet holds its delegate weakly.
  private var storePageDelegate: StorePageDelegate?

  public func definition() -> ModuleDefinition {
    Name("Portcullis")

    /// { platform: "ios", environment: "appstore" | "testflight" | "simulator" | "development" | "unknown" }
    AsyncFunction("getInstallSourceAsync") { () async -> [String: String] in
      return ["platform": "ios", "environment": await Self.environment()]
    }

    /// Shows the app's own App Store page in a sheet, so people can update
    /// without leaving the app. The id is looked up from the bundle id when
    /// the app doesn't pass one. Resolves false if it couldn't be shown.
    AsyncFunction("openAppStorePageAsync") { (appStoreId: String?) async -> Bool in
      guard let id = await Self.resolveAppStoreId(appStoreId) else { return false }
      return await self.presentStorePage(id: id)
    }
  }

  // MARK: - Install environment

  /// Never calls AppTransaction.refresh(): that shows an App Store sign-in
  /// prompt, which a gate must never do at launch. `shared` doesn't prompt;
  /// it throws instead, and a throw here means "unknown", so no store.
  /// AppTransaction needs iOS 16; on iOS 15 the answer is "unknown" too.
  static func environment() async -> String {
    #if targetEnvironment(simulator)
    return "simulator"
    #else
    guard #available(iOS 16.0, *) else { return "unknown" }
    do {
      guard case .verified(let transaction) = try await AppTransaction.shared else { return "unknown" }
      switch transaction.environment {
      case .production: return "appstore"
      // Sandbox signs TestFlight builds, and development builds run outside
      // Xcode. Either way there's no store to send anyone to.
      case .sandbox: return "testflight"
      case .xcode: return "development"
      default: return "unknown"
      }
    } catch {
      return "unknown"
    }
    #endif
  }

  // MARK: - App Store page

  /// The numeric App Store id: the one given, or Apple's lookup by bundle id.
  static func resolveAppStoreId(_ given: String?) async -> String? {
    if let given, !given.isEmpty, given.allSatisfy(\.isNumber) { return given }
    guard
      let bundleId = Bundle.main.bundleIdentifier,
      var lookup = URLComponents(string: "https://itunes.apple.com/lookup")
    else { return nil }
    lookup.queryItems = [URLQueryItem(name: "bundleId", value: bundleId)]
    guard
      let url = lookup.url,
      let (data, _) = try? await URLSession.shared.data(from: url),
      let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
      let first = (json["results"] as? [[String: Any]])?.first,
      let trackId = first["trackId"] as? Int
    else { return nil }
    return String(trackId)
  }

  @MainActor
  private func presentStorePage(id: String) async -> Bool {
    guard let presenter = appContext?.utilities?.currentViewController() else {
      return await openStoreURL(id: id)
    }
    let store = SKStoreProductViewController()
    let delegate = StorePageDelegate { [weak self] in self?.storePageDelegate = nil }
    storePageDelegate = delegate
    store.delegate = delegate
    do {
      try await store.loadProduct(withParameters: [SKStoreProductParameterITunesItemIdentifier: id])
    } catch {
      return await openStoreURL(id: id)
    }
    presenter.present(store, animated: true)
    return true
  }

  /// The fallback: the App Store app itself, at this app's page.
  @MainActor
  private func openStoreURL(id: String) async -> Bool {
    guard let url = URL(string: "itms-apps://apps.apple.com/app/id\(id)") else { return false }
    return await UIApplication.shared.open(url)
  }
}

/// Dismisses the App Store sheet when the person closes it.
private final class StorePageDelegate: NSObject, SKStoreProductViewControllerDelegate {
  private let onFinish: () -> Void

  init(onFinish: @escaping () -> Void) {
    self.onFinish = onFinish
  }

  func productViewControllerDidFinish(_ viewController: SKStoreProductViewController) {
    viewController.dismiss(animated: true)
    onFinish()
  }
}
