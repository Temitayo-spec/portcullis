package expo.modules.portcullis

import android.app.Activity
import android.content.Context
import android.os.Build
import com.google.android.play.core.appupdate.AppUpdateManagerFactory
import com.google.android.play.core.appupdate.AppUpdateOptions
import com.google.android.play.core.install.model.AppUpdateType
import com.google.android.play.core.install.model.UpdateAvailability
import expo.modules.kotlin.Promise
import expo.modules.kotlin.exception.Exceptions
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * The Android half of expo-portcullis: which app installed this one
 * (spec/gate-v1.md R8), and Play's own update screen.
 *
 * It reports raw facts only. Turning the installer into a store key happens
 * in JavaScript (storeKeyFor), where the shared spec cases test it.
 */
class PortcullisModule : Module() {
  private val context: Context
    get() = appContext.reactContext ?: throw Exceptions.ReactContextLost()

  override fun definition() = ModuleDefinition {
    Name("Portcullis")

    /** { platform: "android", installer: "com.android.vending" | ... | null } */
    AsyncFunction("getInstallSourceAsync") {
      mapOf("platform" to "android", "installer" to installerPackage())
    }

    /**
     * Play's full-screen "update now" flow. Resolves how it ended:
     * "accepted", "cancelled", "not-available", "not-allowed", "failed" or
     * "no-activity". Anything but accepted or cancelled means the caller
     * should fall back to opening the Play listing.
     */
    AsyncFunction("startPlayUpdateAsync") { promise: Promise ->
      val activity = appContext.currentActivity
      if (activity == null) {
        promise.resolve("no-activity")
        return@AsyncFunction
      }
      val manager = AppUpdateManagerFactory.create(activity)
      manager.appUpdateInfo
        .addOnSuccessListener { info ->
          val availability = info.updateAvailability()
          val available = availability == UpdateAvailability.UPDATE_AVAILABLE ||
            // An update this app already started: resume it.
            availability == UpdateAvailability.DEVELOPER_TRIGGERED_UPDATE_IN_PROGRESS
          when {
            !available -> promise.resolve("not-available")
            !info.isUpdateTypeAllowed(AppUpdateType.IMMEDIATE) -> promise.resolve("not-allowed")
            else -> manager
              .startUpdateFlow(info, activity, AppUpdateOptions.newBuilder(AppUpdateType.IMMEDIATE).build())
              .addOnSuccessListener { code ->
                promise.resolve(
                  when (code) {
                    Activity.RESULT_OK -> "accepted"
                    Activity.RESULT_CANCELED -> "cancelled"
                    else -> "failed"
                  }
                )
              }
              .addOnFailureListener { promise.resolve("failed") }
          }
        }
        // Not a Play install, no Play Store, or offline: Play can't help here.
        .addOnFailureListener { promise.resolve("failed") }
    }
  }

  /**
   * The package that actually installed this app, as the system recorded it.
   * Android 11 and later: InstallSourceInfo.installingPackageName, which the
   * installer can't fake. Earlier: the older getInstallerPackageName.
   */
  private fun installerPackage(): String? = try {
    val packageManager = context.packageManager
    val packageName = context.packageName
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
      packageManager.getInstallSourceInfo(packageName).installingPackageName
    } else {
      @Suppress("DEPRECATION")
      packageManager.getInstallerPackageName(packageName)
    }
  } catch (e: Exception) {
    null
  }
}
