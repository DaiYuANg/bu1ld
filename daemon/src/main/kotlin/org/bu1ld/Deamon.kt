package org.bu1ld

import io.github.oshai.kotlinlogging.KotlinLogging
import io.smallrye.mutiny.Uni
import io.smallrye.mutiny.coroutines.awaitSuspending
import io.smallrye.mutiny.coroutines.uni
import io.vertx.mutiny.core.Vertx
import kotlinx.coroutines.flow.combine
import org.bu1ld.verticle.HttpServerVerticle
import org.bu1ld.verticle.SockVerticle
import org.bu1ld.verticle.VerticleModule
import org.koin.core.component.KoinComponent
import org.koin.core.component.inject
import org.koin.core.context.startKoin
import org.koin.dsl.module
import org.koin.ksp.generated.module


val appModule = module {
  single {
    Vertx.vertx()
  }
}

class App : KoinComponent {
  val vertx: Vertx by inject()
  val httpVerticle: HttpServerVerticle by inject()
  val sockVerticle: SockVerticle by inject()
}

suspend fun main() {
  startKoin {
    printLogger()
    modules(appModule, VerticleModule().module)
  }

  val app = App()
  val vertx = app.vertx
  // 依赖 Vertx 的 deployVerticle 是异步的，用协程挂起等待
  val deploy1 = app.vertx.deployVerticle(app.httpVerticle)
  val deploy2 = app.vertx.deployVerticle(app.sockVerticle)
  Uni.combine().all().unis(deploy1, deploy2).asTuple().awaitSuspending()

}