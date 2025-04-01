
import React, { useState, useEffect, createContext, useContext } from 'react';
import { createRoot } from 'react-dom/client';
import { 
  createBrowserRouter,
  RouterProvider,
  Outlet,
  useNavigate,
  useLocation,
  Navigate
} from 'react-router';
import { 
  Layout, Menu, Button, Table, Form, Input, 
  Select, DatePicker, Card, Statistic, 
  Typography, Space, Tag, Modal, message,
  Spin, Row, Col, Breadcrumb, Avatar,
  Dropdown, ConfigProvider, theme,
  Switch, Badge
} from 'antd'; 
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
  DashboardOutlined,
  TeamOutlined,
  SettingOutlined,
  LogoutOutlined,
  BellOutlined
} from '@ant-design/icons';     
import { 
  QueryClient,
  QueryClientProvider,
  useQuery,
  useMutation,
  useQueryClient
} from '@tanstack/react-query';
import axios from 'axios';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';

// 设置 dayjs 语言
dayjs.locale('zh-cn');

const { Header, Sider, Content } = Layout;
const { Title } = Typography;

// 创建QueryClient实例
const queryClient = new QueryClient();

// 定义类型
interface User {
  id: number;
  username: string;
  nickname?: string;
  email?: string;
  phone?: string;
  role: string;
  avatar?: string;
}

interface MenuItem {
  id: number;
  name: string;
  path: string;
  icon: string;
  component: string;
  parent_id?: number;
  children?: MenuItem[];
}

// 认证上下文类型
interface AuthContextType {
  user: User | null;
  token: string | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  isAuthenticated: boolean;
  isLoading: boolean;
}

// 主题上下文类型
interface ThemeContextType {
  isDark: boolean;
  toggleTheme: () => void;
}

// 创建上下文
const AuthContext = createContext<AuthContextType | null>(null);
const ThemeContext = createContext<ThemeContextType | null>(null);

// 认证提供器组件
const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(localStorage.getItem('token'));
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const queryClient = useQueryClient();
  
  // 使用useQuery检查登录状态
  const { isLoading } = useQuery({
    queryKey: ['auth', 'status', token],
    queryFn: async () => {
      if (!token) {
        setIsAuthenticated(false);
        setUser(null);
        return null;
      }
      
      try {
        const response = await axios.get('/api/auth/status', {
          headers: { Authorization: `Bearer ${token}` }
        });
        
        if (response.data.isValid) {
          setUser(response.data.user);
          setIsAuthenticated(true);
        } else {
          handleLogout();
        }
        return response.data;
      } catch (error) {
        console.error('验证登录状态失败:', error);
        handleLogout();
        throw error;
      }
    },
    // 只在有token时才执行查询
    enabled: !!token,
    // 不要自动重新获取，让我们自己控制何时刷新
    refetchOnWindowFocus: false,
    retry: false
  });
  
  // 设置请求拦截器
  useEffect(() => {
    axios.interceptors.request.use(
      (config) => {
        if (token) {
          config.headers.Authorization = `Bearer ${token}`;
        }
        return config;
      },
      (error) => {
        return Promise.reject(error);
      }
    );
    
    axios.interceptors.response.use(
      (response) => response,
      (error) => {
        if (error.response?.status === 401) {
          handleLogout();
      }
      return Promise.reject(error);
    });
  }, [token]);
  
  const handleLogin = async (username: string, password: string) => {
    const response = await axios.post('/api/auth/login', { username, password });
    const { token: newToken, user: userData } = response.data;
    
    setToken(newToken);
    setUser(userData);
    setIsAuthenticated(true);
    localStorage.setItem('token', newToken);
  };
  
  const handleLogout = async () => {
    try {
      await axios.post('/api/auth/logout');
    } catch (error) {
      console.error('登出请求失败:', error);
    } finally {
      setToken(null);
      setUser(null);
      setIsAuthenticated(false);
      localStorage.removeItem('token');
      // 清除所有查询缓存
      queryClient.clear();
    }
  };
  
  return (
    <AuthContext.Provider 
      value={{
        user,
        token,
        login: handleLogin,
        logout: handleLogout,
        isAuthenticated,
        isLoading
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

// 主题提供器组件
const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [isDark, setIsDark] = useState(false);
  
  const toggleTheme = () => {
    setIsDark(!isDark);
    document.body.classList.toggle('dark');
  };
  
  return (
    <ThemeContext.Provider value={{ isDark, toggleTheme }}>
      <ConfigProvider
        theme={{
          algorithm: isDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
        }}
      >
        {children}
      </ConfigProvider>
    </ThemeContext.Provider>
  );
};

// 使用上下文的钩子
const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth必须在AuthProvider内部使用');
  }
  return context;
};

const useTheme = () => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme必须在ThemeProvider内部使用');
  }
  return context;
};

// 登录页面
const LoginPage = () => {
  const { login } = useAuth();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  
  const handleSubmit = async (values: { username: string; password: string }) => {
    try {
      setLoading(true);
      await login(values.username, values.password);
      // 登录成功后跳转到管理后台首页
      navigate('/admin/dashboard');
    } catch (error: any) {
      message.error(error.response?.data?.error || '登录失败');
    } finally {
      setLoading(false);
    }
  };
  
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-md w-full space-y-8">
        <div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            登录管理后台
          </h2>
        </div>
        
        <Card>
          <Form
            form={form}
            name="login"
            onFinish={handleSubmit}
            autoComplete="off"
            layout="vertical"
          >
            <Form.Item
              name="username"
              rules={[{ required: true, message: '请输入用户名' }]}
            >
              <Input 
                prefix={<UserOutlined />} 
                placeholder="用户名" 
                size="large"
              />
            </Form.Item>
            
            <Form.Item
              name="password"
              rules={[{ required: true, message: '请输入密码' }]}
            >
              <Input.Password 
                placeholder="密码" 
                size="large"
              />
            </Form.Item>
            
            <Form.Item>
              <Button 
                type="primary" 
                htmlType="submit" 
                size="large" 
                block
                loading={loading}
              >
                登录
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </div>
    </div>
  );
};

// 仪表盘页面
const DashboardPage = () => {
  return (
    <div>
      <Title level={2}>仪表盘</Title>
      <Row gutter={16}>
        <Col span={8}>
          <Card>
            <Statistic
              title="活跃用户"
              value={112893}
              loading={false}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="系统消息"
              value={93}
              loading={false}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="在线用户"
              value={1128}
              loading={false}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

// 用户管理页面
const UsersPage = () => {
  const { data: users = [], isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: async () => {
      const response = await axios.get('/api/auth/users');
      return response.data;
    }
  });
  
  const columns = [
    {
      title: '用户名',
      dataIndex: 'username',
      key: 'username',
    },
    {
      title: '昵称',
      dataIndex: 'nickname',
      key: 'nickname',
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      key: 'email',
    },
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      render: (role: string) => (
        <Tag color={role === 'admin' ? 'red' : 'blue'}>
          {role === 'admin' ? '管理员' : '普通用户'}
        </Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => dayjs(date).format('YYYY-MM-DD HH:mm:ss'),
    },
  ];
  
  return (
    <div>
      <Title level={2}>用户管理</Title>
      <Card>
        <Table
          columns={columns}
          dataSource={users}
          loading={isLoading}
          rowKey="id"
        />
      </Card>
    </div>
  );
};

// 系统设置页面
const SettingsPage = () => {
  const { isDark, toggleTheme } = useTheme();
  
  return (
    <div>
      <Title level={2}>系统设置</Title>
      <Card title="主题设置">
        <Space direction="vertical" size="middle">
          <div>
            <span className="mr-4">深色模式</span>
            <Switch 
              checked={isDark}
              onChange={toggleTheme}
            />
          </div>
        </Space>
      </Card>
    </div>
  );
};

// 图标映射
const icons = {
  dashboard: DashboardOutlined,
  user: UserOutlined,
  setting: SettingOutlined
};

// 主布局组件
const MainLayout = () => {
  const { user, logout } = useAuth();
  const [collapsed, setCollapsed] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const OutletComponent = Outlet as any;
  
  // 获取菜单数据
  const { data: menuItems = [] } = useQuery({
    queryKey: ['menus'],
    queryFn: async () => {
      const response = await axios.get('/api/auth/menus');
      return response.data;
    }
  });
  
  // 处理菜单项点击
  const handleMenuClick = ({ key }: { key: string }) => {
    navigate(`/admin${key}`);
  };
  
  // 处理登出
  const handleLogout = async () => {
    await logout();
    navigate('/admin/login');
  };
  
  // 用户下拉菜单项
  const userMenuItems = [
    {
      key: 'profile',
      label: '个人信息',
      icon: <UserOutlined />
    },
    {
      key: 'logout',
      label: '退出登录',
      icon: <LogoutOutlined />,
      onClick: handleLogout
    }
  ];
  
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider trigger={null} collapsible collapsed={collapsed}>
        <div className="p-4">
          <h1 className="text-white text-xl font-bold">
            {collapsed ? 'Admin' : '管理后台'}
          </h1>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems.map(item => ({
            key: item.path,
            icon: React.createElement(icons[item.icon] || DashboardOutlined),
            label: item.name,
          }))}
          onClick={handleMenuClick}
        />
      </Sider>
      
      <Layout>
        <Header className="bg-white p-0 flex justify-between items-center">
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setCollapsed(!collapsed)}
            className="w-16 h-16"
          />
          
          <Space size="middle" className="mr-4">
            <Badge count={5}>
              <Button type="text" icon={<BellOutlined />} />
            </Badge>
            
            <Dropdown menu={{ items: userMenuItems }}>
              <Space className="cursor-pointer">
                <Avatar 
                  src={user?.avatar}
                  icon={!user?.avatar && <UserOutlined />}
                />
                <span>{user?.nickname || user?.username}</span>
              </Space>
            </Dropdown>
          </Space>
        </Header>
        
        <Content className="m-6">
          <div className="site-layout-content bg-white p-6 rounded-lg">
            <OutletComponent />
          </div>
        </Content>
      </Layout>
    </Layout>
  );
};

// 受保护的路由组件
const ProtectedRoute = ({ children }: { children: React.ReactNode }) => {
  const { isAuthenticated, isLoading } = useAuth();
  const navigate = useNavigate();
  
  useEffect(() => {
    // 只有在加载完成且未认证时才重定向
    if (!isLoading && !isAuthenticated) {
      navigate('/admin/login', { replace: true });
    }
  }, [isAuthenticated, isLoading, navigate]);
  
  // 显示加载状态，直到认证检查完成
  if (isLoading) {
    return (
      <div className="flex justify-center items-center h-screen">
        <div className="loader ease-linear rounded-full border-4 border-t-4 border-gray-200 h-12 w-12"></div>
      </div>
    );
  }
  
  // 如果未认证且不再加载中，不显示任何内容（等待重定向）
  if (!isAuthenticated) {
    return null;
  }
  
  return children;
};


// 主应用组件
const App = () => {
  const RouterProviderComponent = RouterProvider as any;
  // 创建路由
  const router = createBrowserRouter([
    {
      path: '/admin/login',
      element: <LoginPage />
    },
    {
      path: '/admin',
      element: (
        <ProtectedRoute>
          <MainLayout />
        </ProtectedRoute>
      ),
      children: [
        {
          index: true,
          element: <Navigate to="/admin/dashboard" />
        },
        {
          path: 'dashboard',
          element: <DashboardPage />
        },
        {
          path: 'users',
          element: <UsersPage />
        },
        {
          path: 'settings',
          element: <SettingsPage />
        }
      ]
    }
  ]);
  return <RouterProviderComponent router={router} />
};

// 渲染应用
const root = createRoot(document.getElementById('root') as HTMLElement);
root.render(
  <QueryClientProvider client={queryClient}>
    <ThemeProvider>
      <AuthProvider>
        <App />
      </AuthProvider>
    </ThemeProvider>
  </QueryClientProvider>
);