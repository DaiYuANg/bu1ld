package org.bu1ld.command

import com.github.ajalt.clikt.core.CliktCommand
import com.github.ajalt.clikt.core.PrintHelpMessage
import org.bu1ld.rootModule
import org.koin.core.context.startKoin

class Root : CliktCommand() {
  val module = startKoin {
    rootModule
  }

  override fun run() {
    throw PrintHelpMessage(currentContext) as Throwable
  }
}
