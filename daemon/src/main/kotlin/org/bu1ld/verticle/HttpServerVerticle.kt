package org.bu1ld.verticle

import io.github.oshai.kotlinlogging.KotlinLogging
import io.smallrye.mutiny.Uni
import io.smallrye.mutiny.coroutines.awaitSuspending
import io.smallrye.mutiny.replaceWithUnit
import io.smallrye.mutiny.vertx.core.AbstractVerticle
import org.koin.core.annotation.Single

@Single
class HttpServerVerticle : AbstractVerticle() {
  private val logger = KotlinLogging.logger {}
  override fun asyncStart(): Uni<Void> {
    logger.atInfo { message = "start http server" }
    return vertx.createHttpServer()
      .requestHandler { req -> req.response().end("Hello from Vert.x + Koin!") }
      .listen().replaceWithUnit()
      .invoke { unit ->
        {
          logger.atInfo { message = "start http server" }
        }
      }
      .replaceWithVoid()
  }
}