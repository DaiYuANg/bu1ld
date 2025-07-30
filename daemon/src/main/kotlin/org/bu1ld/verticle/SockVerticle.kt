package org.bu1ld.verticle

import io.github.oshai.kotlinlogging.KotlinLogging
import io.smallrye.mutiny.Uni
import io.smallrye.mutiny.replaceWithUnit
import io.smallrye.mutiny.vertx.core.AbstractVerticle
import org.koin.core.annotation.Single

@Single
class SockVerticle : AbstractVerticle() {
  private val logger = KotlinLogging.logger {}
  override fun asyncStart(): Uni<Void> {
    return vertx.createNetServer().connectHandler {  }.listen().replaceWithUnit().replaceWithVoid()
  }
}