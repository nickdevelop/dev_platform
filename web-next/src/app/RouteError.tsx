import { isRouteErrorResponse, useRouteError } from 'react-router'

export function RouteError() {
  const error = useRouteError()
  const message = isRouteErrorResponse(error)
    ? `${error.status} ${error.statusText}`
    : error instanceof Error
      ? error.message
      : '未知路由错误'

  return (
    <main className="p0-login" role="alert">
      <h1>页面加载失败</h1>
      <p>{message}</p>
      <a href="/spike">返回验证页</a>
    </main>
  )
}
