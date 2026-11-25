<script setup lang="ts">
import { RouterView } from 'vue-router'
// zhCn is the Element Plus locale for every widget's built-in copy (pagination,
// empty table, date picker, transfer). The platform UI is Chinese-only, so the
// locale is set once for the whole tree here in the composition root instead of
// per dialog.
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>

<template>
  <el-config-provider :locale="zhCn">
    <RouterView />
  </el-config-provider>
</template>
