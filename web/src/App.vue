<script setup lang="ts">
import { RouterView } from 'vue-router'
// zhCn is the Element Plus locale for every widget's built-in copy (pagination,
// empty table, date picker, transfer). The platform UI is Chinese-only, so the
// locale is set once for the whole tree here in the composition root instead of
// per dialog.
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>

<template>
  <!-- `dialog.alignCenter` is the platform's one dialog-placement rule for every
       el-dialog in the app (spec §四): panels sit in the middle of the viewport
       instead of 15vh from the top, and the token layer caps their height so a
       long form scrolls inside its own body with the footer pinned. -->
  <el-config-provider :locale="zhCn" :dialog="{ alignCenter: true }">
    <RouterView />
  </el-config-provider>
</template>
