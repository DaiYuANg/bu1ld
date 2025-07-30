package org.bu1ld

import com.github.ajalt.clikt.core.main
import org.bu1ld.command.Root
import org.koin.core.context.startKoin
import org.koin.dsl.module


val rootModule = module {

}

fun main(args: Array<String>) {
    val module = startKoin {

    }
    Root().main(args)
}