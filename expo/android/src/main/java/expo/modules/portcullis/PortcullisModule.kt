package expo.modules.portcullis

import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

class PortcullisModule : Module() {
  override fun definition() = ModuleDefinition {
    Name("Portcullis")

    AsyncFunction("setValueAsync") { value: String ->
    }
  }
}
