import ExpoModulesCore

public class PortcullisModule: Module {
  public func definition() -> ModuleDefinition {
    Name("Portcullis")

    AsyncFunction("setValueAsync") { (value: String) in
    }
  }
}
